package fastroute

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestRecyclesParameters(t *testing.T) {
	t.Parallel()

	router := New("/users/:id", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("not expected invocation")
	})

	req, _ := http.NewRequest("GET", "/users/5", nil)
	if h := router.Route(req); h == nil {
		t.Fatalf("expected request for path: %s to be routed, but it was not", req.URL.Path)
	}

	if len(Parameters(req)) != 1 {
		t.Fatalf("expected one parameter, but there was: %+v", Parameters(req))
	}

	Recycle(req)

	if len(Parameters(req)) != 0 {
		t.Fatal("should have recycled parameters")
	}
	if _, ok := req.Body.(*parameters); ok {
		t.Fatal("should have reset request body")
	}
}

func TestShouldFallbackToNotFoundHandler(t *testing.T) {
	t.Parallel()
	router := New("/xx", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("not expected invocation")
	})
	req, err := http.NewRequest("GET", "/any", nil)
	w := httptest.NewRecorder()
	if err != nil {
		t.Fatal(err)
	}

	router.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("unexpected response code: %d", w.Code)
	}
}

func TestShouldRouteRouterAsHandler(t *testing.T) {
	t.Parallel()
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}

	routes := map[string]Router{
		"GET":  New("/users", handler),
		"POST": New("/users/:id", handler),
	}

	router := RouterFunc(func(req *http.Request) http.Handler {
		return routes[req.Method] // fastroute.Router is also http.Handler
	})

	app := Chain(router, New("/any", handler))

	req, _ := http.NewRequest("GET", "/any", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("unexpected response code: %d", w.Code)
	}

	req, _ = http.NewRequest("GET", "/users/1", nil)
	w = httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("unexpected response code: %d", w.Code)
	}

	req, _ = http.NewRequest("PUT", "/something", nil)
	w = httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("unexpected response code: %d", w.Code)
	}
}

func TestEmptyRequestParameters(t *testing.T) {
	t.Parallel()
	req, err := http.NewRequest("GET", "/any", nil)
	if err != nil {
		t.Fatal(err)
	}

	params := Parameters(req)
	if len(params) > 0 {
		t.Fatalf("expected empty params, but got: %d", len(params))
	}

	if act := params.ByName("unknown"); act != "" {
		t.Fatalf("expected empty value for unknown param, but got: %s", act)
	}
}

func TestRoutePatternValidation(t *testing.T) {
	t.Parallel()
	recoverOrFail(
		"/path/*",
		"param must be named after sign: /path/*",
		http.NotFoundHandler(),
		t,
	)

	recoverOrFail(
		"/path/:/a",
		"param must be named after sign: /path/:/a",
		http.NotFoundHandler(),
		t,
	)

	recoverOrFail(
		"/pa:/a",
		"special param matching signs, must follow after slash: /pa:/a",
		http.NotFoundHandler(),
		t,
	)

	recoverOrFail(
		"/a/b*",
		"special param matching signs, must follow after slash: /a/b*",
		http.NotFoundHandler(),
		t,
	)

	recoverOrFail(
		"/:user:/id",
		"only one param per segment: /:user:/id",
		http.NotFoundHandler(),
		t,
	)

	recoverOrFail(
		"/a/b*",
		"not a handler given: string - MyHandler",
		"MyHandler",
		t,
	)

	recoverOrFail(
		"/path/*all/more",
		"match all, must be the last segment in pattern: /path/*all/more",
		http.NotFoundHandler(),
		t,
	)

	recoverOrFail("/path", "given handler cannot be: nil", nil, t)
}

func TestStaticRouteMatcher(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"/users/hello":      true,
		"/user/hello":       false,
		"/users/hello/":     false,
		"/users/hello/bin":  false,
		"/users/hello/bin/": true,
		"/":                 true,
	}
	router := Chain(
		New("/users/hello/bin/", http.NotFoundHandler()),
		New("/", http.NotFoundHandler()),
		New("/users/hello", func(w http.ResponseWriter, r *http.Request) {}),
	)

	for path, matched := range cases {
		req, err := http.NewRequest("GET", path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if matched && router.Route(req) == nil {
			t.Fatalf("expected to match: %s", path)
		}
		if !matched && router.Route(req) != nil {
			t.Fatalf("did not expect to match: %s", path)
		}

		pat := Pattern(req)
		if pat != path && matched {
			t.Fatalf("expected matched pattern: %s is not the same: %s", pat, path)
		}

		params := Parameters(req)
		if len(params) > 0 {
			t.Fatal("expected empty params")
		}
	}
}

func TestDynamicRouteMatcher(t *testing.T) {
	t.Parallel()
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, "OK")
	})
	router := Chain(
		New("/a/:b/c", handler),
		New("/category/:cid/product/*rest", handler),
		New("/users/:id/:bid/", handler),
		New("/applications/:client_id/tokens", handler),
		New("/repos/:owner/:repo/issues/:number/labels/:name", handler),
		New("/files/*filepath", handler),
		New("/hello/:name", handler),
		New("/search/:query", handler),
		New("/search/", handler),
		New("/ünìcodé.html", handler),
	)

	type kv map[string]string // reduce clutter

	cases := []struct {
		path    string
		pattern string
		params  kv
		match   bool
	}{
		{"/hello/john", "/hello/:name", kv{"name": "john"}, true},
		{"/hellowe", "/hellowe", kv{}, false},
		{"/a/dic/c", "/a/:b/c", kv{"b": "dic"}, true},
		{"/a/d/c", "/a/:b/c", kv{"b": "d"}, true},
		{"/a/c", "/a/c", kv{}, false},
		{"/a/c/c", "/a/:b/c", kv{"b": "c"}, true},
		{"/a/c/b", "/a/c/b", kv{}, false},
		{"/a/c/c/", "/a/c/c/", kv{}, false},
		{"/category/5/product/x/a/bc", "/category/:cid/product/*rest", kv{"cid": "5", "rest": "/x/a/bc"}, true},
		{"/users/a/b/", "/users/:id/:bid/", kv{"id": "a", "bid": "b"}, true},
		{"/users/a/b/be/", "/users/a/b/be/", kv{}, false},
		{"/applications/:client_id/tokens", "/applications/:client_id/tokens", kv{"client_id": ":client_id"}, true},
		{"/repos/:owner/:repo/issues/:number/labels", "/repos/:owner/:repo/issues/:number/labels", kv{}, false},
		{"/files/LICENSE", "/files/*filepath", kv{"filepath": "/LICENSE"}, true},
		{"/files/", "/files/*filepath", kv{"filepath": "/"}, true},
		{"/files", "/files", kv{}, false},
		{"/files/css/style.css", "/files/*filepath", kv{"filepath": "/css/style.css"}, true},
		{"/search/", "/search/", kv{}, true},
		{"/search", "/search", kv{}, false},
		{"/search/someth!ng+in+ünìcodé", "/search/:query", kv{"query": "someth!ng+in+ünìcodé"}, true},
		{"/search/someth!ng+in+ünìcodé/", "/search/someth!ng+in+ünìcodé/", kv{}, false},
		{"/ünìcodé.html", "/ünìcodé.html", kv{}, true},
	}

	for i, c := range cases {
		req, err := http.NewRequest("GET", c.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		h := router.Route(req)
		if c.match && h == nil {
			t.Fatalf("expected to match: %s", c.path)
		}
		if !c.match && h != nil {
			t.Fatalf("did not expect to match: %s", c.path)
		}

		pat := Pattern(req)
		if pat != c.pattern {
			t.Fatalf("expected matched pattern: %s does not match to: %s, case: %d", c.pattern, pat, i)
		}

		params := Parameters(req)
		for key, val := range c.params {
			act := params.ByName(key)
			if act != val {
				t.Fatalf("param: %s expected %s does not match to: %s, case: %d", key, val, act, i)
			}
		}

		if h == nil {
			continue
		}

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Body.String() != "OK" || w.Code != 200 {
			t.Fatal("not expected response body or code")
		}

		if params := Parameters(req); len(params) != 0 {
			t.Fatal("parameters should have been flushed")
		}
	}
}

func TestGenerated(t *testing.T) {
	routes, pat := generateRoutes(60, 5)
	pat = strings.Replace(pat, ":id", "param", 1)

	router := Chain(routes...)

	req, err := http.NewRequest("GET", pat, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatal("not expected response code")
	}
}

func Benchmark_1Param(b *testing.B) {
	router := New("/v1/users/:id", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(Parameters(r).ByName("id")))
	})

	req, err := http.NewRequest("GET", "/v1/users/5", nil)
	if err != nil {
		b.Fatal(err)
	}

	benchmark(b, router, req)
}

func Benchmark_Static(b *testing.B) {
	router := New("/static/path/pattern", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	req, err := http.NewRequest("GET", "/static/path/pattern", nil)
	if err != nil {
		b.Fatal(err)
	}

	benchmark(b, router, req)
}

func Benchmark_5Routes(b *testing.B) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(Parameters(r).ByName("id")))
	}

	router := Chain(
		New("/test/:id", handler),
		New("/puff/path/:id", handler),
		New("/home/user/:id", handler),
		New("/home/jey/:id/:cat", handler),
		New("/base/:id/user", handler),
	)

	req, err := http.NewRequest("GET", "/home/jey/5/user", nil)
	if err != nil {
		b.Fatal(err)
	}

	benchmark(b, router, req)
}

func Benchmark_1000Routes_1Param(b *testing.B) {
	routes, pat := generateRoutes(1000, 10)
	pat = strings.Replace(pat, ":id", "param", 1)

	router := Chain(routes...)

	req, err := http.NewRequest("GET", pat, nil)
	if err != nil {
		b.Fatal(err)
	}

	benchmark(b, router, req)
}

func Benchmark_1000Routes_1Param_HitCounting(b *testing.B) {
	routes, pat := generateRoutes(1000, 10)
	pat = strings.Replace(pat, ":id", "param", 1)

	router := HitCountingOrderedChain(routes...)

	req, err := http.NewRequest("GET", pat, nil)
	if err != nil {
		b.Fatal(err)
	}

	benchmark(b, router, req)
}

func HitCountingOrderedChain(routes ...Router) Router {
	hitRoutes := make([]*HitCounter, len(routes))
	for i, r := range routes {
		hitRoutes[i] = &HitCounter{Router: r}
	}

	return RouterFunc(func(req *http.Request) http.Handler {
		for i, r := range hitRoutes {
			if h := r.Route(req); h != nil {
				r.hits++
				// reorder route hit is behind one third of routes
				if i > len(hitRoutes)*30/100 {
					sort.Sort(SortByHits(hitRoutes))
				}
				return h
			}
		}
		return nil
	})
}

type HitCounter struct {
	Router
	hits int64
}

type SortByHits []*HitCounter

func (s SortByHits) Len() int           { return len(s) }
func (s SortByHits) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s SortByHits) Less(i, j int) bool { return s[i].hits > s[j].hits }

func recoverOrFail(pattern, expectedMessage string, h interface{}, t *testing.T) {
	defer func() {
		if err := recover(); err != nil {
			actual := fmt.Sprintf("%s", err)
			if actual != expectedMessage {
				t.Fatalf(`actual message: "%s" does not match expected: "%s"`, actual, expectedMessage)
			}
		}
	}()

	New(pattern, h)

	t.Fatalf(`was expecting pattern: "%s" to panic with message: "%s"`, pattern, expectedMessage)
}

func generateRoutes(num, segments int) (routes []Router, last string) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(Parameters(r).ByName("id")))
	}

	alphabet := "abcdefghijklmnopqrstuvwxyz"
	unique := make(map[string]bool)
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	char := func() byte {
		return alphabet[rnd.Intn(len(alphabet)-1)]
	}

	var j int
	for len(unique) < num {
		var segs []string
		for i := 0; i < segments; i++ {
			segs = append(segs, string(char()))
		}

		path := "/" + strings.Join(segs, "/") + "/:id"
		if _, duplicate := unique[path]; !duplicate {
			unique[path] = true
			last = path
			routes = append(routes, New(path, handler))
		}
		j++
	}

	return
}

func benchmark(b *testing.B, router Router, req *http.Request) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(req)
		Recycle(req)
	}
}
