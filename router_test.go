package fastroute_test

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/fastroute"
)

func Example() {
	handler := func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Hello, %s", fastroute.Parameters(req).ByName("name"))
	}

	var routes = map[string]fastroute.Router{
		"GET": fastroute.Chain(
			fastroute.New("/", handler),
			fastroute.New("/hello/:name/:surname", handler),
			fastroute.New("/hello/:name", handler),
		),
		"POST": fastroute.Chain(
			fastroute.New("/users", handler),
			fastroute.New("/users/:name", handler),
		),
	}

	http.ListenAndServe(":8080", fastroute.RouterFunc(func(req *http.Request) http.Handler {
		return routes[req.Method] // fastroute.Router is also http.Handler
	}))
}

func ExampleNew() {
	http.ListenAndServe(":8080", fastroute.New("/hello/:name", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Hello, %s", fastroute.Parameters(req).ByName("name"))
	}))
}

func ExampleChain() {
	handler := func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Hello, %s", fastroute.Parameters(req).ByName("name"))
	}

	router := fastroute.Chain(
		fastroute.New("/", handler),
		fastroute.New("/hello/:name", handler),
	)

	thenNotFound := fastroute.RouterFunc(func(req *http.Request) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(404)
			fmt.Fprintln(w, "Ooops, looks like you mistyped the URL:", req.URL.Path)
		})
	})

	router = fastroute.Chain(router, thenNotFound)

	http.ListenAndServe(":8080", router)
}

func ExampleRecycle() {
	handler := func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Hello, %s", fastroute.Parameters(req).ByName("name"))
	}

	router := fastroute.New("/hello/:name", handler)

	req, err := http.NewRequest("GET", "/hello/john", nil)
	if err != nil {
		panic(err) // handle error
	}

	if h := router.Route(req); h != nil {
		// request is routed to handler h
		// now it has parameters
		fmt.Println("Name:", fastroute.Parameters(req).ByName("name"))

		// and pattern matched
		fmt.Println("Pattern:", fastroute.Pattern(req))

		// since parameters are not reallocated, we need to recycle them
		// unless we actually serve this matched handler
		fastroute.Recycle(req)

		// there are no more request parameters or pattern
		fmt.Printf("After recycle name is now empty: '%s'", fastroute.Parameters(req).ByName("name"))
	}
	// Output:
	// Name: john
	// Pattern: /hello/:name
	// After recycle name is now empty: ''
}

func TestRaceConditionForMatchingSingleStaticRoute(t *testing.T) {
	t.Parallel()

	router := fastroute.New("/static/route", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	execFunc := func(wg *sync.WaitGroup) {
		req, _ := http.NewRequest("GET", "/static/route", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal("expected OK status")
		}
		wg.Done()
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go execFunc(&wg)
	}
	wg.Wait()
}

func TestRaceConditionForMatchingSingleDynamicRoute(t *testing.T) {
	t.Parallel()

	router := fastroute.New("/users/:id", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	execFunc := func(wg *sync.WaitGroup) {
		req, _ := http.NewRequest("GET", "/users/5", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal("expected OK status")
		}
		wg.Done()
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go execFunc(&wg)
	}
	wg.Wait()
}

func TestRecyclesParameters(t *testing.T) {
	t.Parallel()

	router := fastroute.New("/users/:id", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("not expected invocation")
	})

	req, _ := http.NewRequest("GET", "/users/5", nil)
	if h := router.Route(req); h == nil {
		t.Fatalf("expected request for path: %s to be routed, but it was not", req.URL.Path)
	}

	if len(fastroute.Parameters(req)) != 1 {
		t.Fatalf("expected one parameter, but there was: %+v", fastroute.Parameters(req))
	}

	fastroute.Recycle(req)

	if len(fastroute.Parameters(req)) != 0 {
		t.Fatal("should have recycled parameters")
	}
}

func TestShouldFallbackToNotFoundHandler(t *testing.T) {
	t.Parallel()
	router := fastroute.New("/xx", func(w http.ResponseWriter, r *http.Request) {
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

func TestShouldParseForm(t *testing.T) {
	t.Parallel()

	route1 := fastroute.New("/ff/:id", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be matched")
	})
	route2 := fastroute.New("/form/:id", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}

		if r.Form.Get("field") != "value" {
			t.Fatal("could not read field")
		}

		params := fastroute.Parameters(r)
		if params == nil || params.ByName("id") != "1" {
			t.Fatal("unexpected id param")
		}
	})

	form := url.Values{}
	form.Add("field", "value")

	req, err := http.NewRequest("POST", "/form/1", strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	if err != nil {
		t.Fatal(err)
	}

	fastroute.Chain(route1, route2).ServeHTTP(w, req)

	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}

	if req.Form.Get("field") != "value" {
		t.Fatal("could not read field")
	}

	params := fastroute.Parameters(req)
	if params != nil {
		t.Fatal("params should be reset after serving request")
	}

	if w.Code != 200 {
		t.Fatalf("unexpected response code: %d", w.Code)
	}
}

func TestShouldRouteRouterAsHandler(t *testing.T) {
	t.Parallel()
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}

	routes := map[string]fastroute.Router{
		"GET":  fastroute.New("/users", handler),
		"POST": fastroute.New("/users/:id", handler),
	}

	router := fastroute.RouterFunc(func(req *http.Request) http.Handler {
		return routes[req.Method] // fastroute.Router is also http.Handler
	})

	app := fastroute.Chain(router, fastroute.New("/any", handler))

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

	params := fastroute.Parameters(req)
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
	router := fastroute.Chain(
		fastroute.New("/users/hello/bin/", http.NotFoundHandler()),
		fastroute.New("/", http.NotFoundHandler()),
		fastroute.New("/users/hello", func(w http.ResponseWriter, r *http.Request) {}),
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

		pat := fastroute.Pattern(req)
		if pat != path && matched {
			t.Fatalf("expected matched pattern: %s is not the same: %s", pat, path)
		}

		params := fastroute.Parameters(req)
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
	router := fastroute.Chain(
		fastroute.New("/a/:b/c", handler),
		fastroute.New("/category/:cid/product/*rest", handler),
		fastroute.New("/users/:id/:bid/", handler),
		fastroute.New("/applications/:client_id/tokens", handler),
		fastroute.New("/repos/:owner/:repo/issues/:number/labels/:name", handler),
		fastroute.New("/files/*filepath", handler),
		fastroute.New("/hello/:name", handler),
		fastroute.New("/search/:query", handler),
		fastroute.New("/search/", handler),
		fastroute.New("/ünìcodé.html", handler),
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

		pat := fastroute.Pattern(req)
		if pat != c.pattern {
			t.Fatalf("expected matched pattern: %s does not match to: %s, case: %d", c.pattern, pat, i)
		}

		params := fastroute.Parameters(req)
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

		if params := fastroute.Parameters(req); len(params) != 0 {
			t.Fatal("parameters should have been flushed")
		}
	}
}

func TestGenerated(t *testing.T) {
	routes, pat := generateRoutes(60, 5)
	pat = strings.Replace(pat, ":id", "param", 1)

	router := fastroute.Chain(routes...)

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
	router := fastroute.New("/v1/users/:id", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fastroute.Parameters(r).ByName("id")))
	})

	req, err := http.NewRequest("GET", "/v1/users/5", nil)
	if err != nil {
		b.Fatal(err)
	}

	benchmark(b, router, req)
}

func Benchmark_Static(b *testing.B) {
	router := fastroute.New("/static/path/pattern", func(w http.ResponseWriter, r *http.Request) {
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
		w.Write([]byte(fastroute.Parameters(r).ByName("id")))
	}

	router := fastroute.Chain(
		fastroute.New("/test/:id", handler),
		fastroute.New("/puff/path/:id", handler),
		fastroute.New("/home/user/:id", handler),
		fastroute.New("/home/jey/:id/:cat", handler),
		fastroute.New("/base/:id/user", handler),
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

	router := fastroute.Chain(routes...)

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

func HitCountingOrderedChain(routes ...fastroute.Router) fastroute.Router {
	hitRoutes := make([]*HitCounter, len(routes))
	for i, r := range routes {
		hitRoutes[i] = &HitCounter{Router: r}
	}

	return fastroute.RouterFunc(func(req *http.Request) http.Handler {
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
	fastroute.Router
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

	fastroute.New(pattern, h)

	t.Fatalf(`was expecting pattern: "%s" to panic with message: "%s"`, pattern, expectedMessage)
}

func generateRoutes(num, segments int) (routes []fastroute.Router, last string) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fastroute.Parameters(r).ByName("id")))
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
			routes = append(routes, fastroute.New(path, handler))
		}
		j++
	}

	return
}

func benchmark(b *testing.B, router fastroute.Router, req *http.Request) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(req)
		fastroute.Recycle(req)
	}
}
