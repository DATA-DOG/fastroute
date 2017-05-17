package fastroute

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestShouldFallbackToNotFoundHandler(t *testing.T) {
	router := Route("/xx", func(w http.ResponseWriter, r *http.Request) {
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

func TestEmptyRequestParameters(t *testing.T) {
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
}

func TestStaticRouteMatcher(t *testing.T) {
	cases := map[string]bool{
		"/users/hello":      true,
		"/user/hello":       false,
		"/users/hello/":     false,
		"/users/hello/bin":  false,
		"/users/hello/bin/": true,
	}
	router := New(
		Route("/users/hello/bin/", http.NotFoundHandler()),
		Route("/users/hello", func(w http.ResponseWriter, r *http.Request) {}),
	)

	for path, matched := range cases {
		req, err := http.NewRequest("GET", path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if matched && router.Match(req) == nil {
			t.Fatalf("expected to match: %s", path)
		}
		if !matched && router.Match(req) != nil {
			t.Fatalf("did not expect to match: %s", path)
		}

		pat := Pattern(req)
		if pat != path && matched {
			t.Fatalf("expected matched pattern: %s is not the same: %s", pat, path)
		}

		params := Parameters(req)
		if !reflect.DeepEqual(emptyParams, params) {
			t.Fatal("expected empty params")
		}
	}
}

func TestCompareBy(t *testing.T) {
	handler := http.NotFoundHandler()

	router := New(
		Route("/status", handler),
		Route("/users/:id", handler),
		Route("/users/:id/roles", handler),
	)
	router = ComparesPathWith(router, strings.EqualFold)

	cases := []string{
		"/staTus",
		"/status",
		"/Users/5",
		"/users/35",
		"/users/2/roles",
		"/Users/2/Roles",
	}

	for _, path := range cases {
		req, err := http.NewRequest("GET", path, nil)
		if err != nil {
			t.Fatal(err)
		}

		h := router.Match(req)
		Recycle(req)
		if h == nil {
			t.Errorf("expected: %s to match", path)
		}
	}
}

func TestDynamicRouteMatcher(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, "OK")
	})
	router := New(
		Route("/a/:b/c", handler),
		Route("/category/:cid/product/*rest", handler),
		Route("/users/:id/:bid/", handler),
		Route("/applications/:client_id/tokens", handler),
		Route("/repos/:owner/:repo/issues/:number/labels/:name", handler),
		Route("/files/*filepath", handler),
	)

	cases := []struct {
		path    string
		pattern string
		params  map[string]string
		match   bool
	}{
		{"/a/dic/c", "/a/:b/c", map[string]string{"b": "dic"}, true},
		{"/a/d/c", "/a/:b/c", map[string]string{"b": "d"}, true},
		{"/a/c", "", map[string]string{}, false},
		{"/a/c/c", "/a/:b/c", map[string]string{"b": "c"}, true},
		{"/a/c/b", "", map[string]string{}, false},
		{"/a/c/c/", "", map[string]string{}, false},
		{"/category/5/product/x/a/bc", "/category/:cid/product/*rest", map[string]string{"cid": "5", "rest": "/x/a/bc"}, true},
		{"/users/a/b/", "/users/:id/:bid/", map[string]string{"id": "a", "bid": "b"}, true},
		{"/users/a/b/be/", "", map[string]string{}, false},
		{"/applications/:client_id/tokens", "/applications/:client_id/tokens", map[string]string{"client_id": ":client_id"}, true},
		{"/repos/:owner/:repo/issues/:number/labels", "", map[string]string{}, false},
		{"/files/LICENSE", "/files/*filepath", map[string]string{"filepath": "/LICENSE"}, true},
		{"/files/", "/files/*filepath", map[string]string{"filepath": "/"}, true},
		{"/files", "", map[string]string{}, false},
		{"/files/css/style.css", "/files/*filepath", map[string]string{"filepath": "/css/style.css"}, true},
	}

	for i, c := range cases {
		req, err := http.NewRequest("GET", c.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		h := router.Match(req)
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

// func TestCaseFix(t *testing.T) {
// 	handler := http.NotFoundHandler()
// 	router := Route("/users/:username/roles", handler)

// 	req, _ := http.NewRequest("GET", "http://127.0.0.1/useRs/gedi/rOles")

// 	before := fastroute.CompareFunc
// 	fastroute.CompareFunc = strings.EqualFold
// 	h := router.Match(req)
// 	fastroute.CompareFunc = before

// 	params := Parameters(req)
// }

func recoverOrFail(pattern, expectedMessage string, h interface{}, t *testing.T) {
	defer func() {
		if err := recover(); err != nil {
			actual := fmt.Sprintf("%s", err)
			if actual != expectedMessage {
				t.Fatalf(`actual message: "%s" does not match expected: "%s"`, actual, expectedMessage)
			}
		}
	}()

	Route(pattern, h)

	t.Fatalf(`was expecting pattern: "%s" to panic with message: "%s"`, pattern, expectedMessage)
}

func Benchmark_1Param(b *testing.B) {
	router := Route("/v1/users/:id", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(Parameters(r).ByName("id")))
	})

	req, err := http.NewRequest("GET", "http://localhost:8080/v1/users/5", nil)
	if err != nil {
		b.Fatal(err)
	}

	benchRequest(b, router, req)
}

func Benchmark_Static(b *testing.B) {
	router := Route("/static/path/pattern", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	req, err := http.NewRequest("GET", "http://localhost:8080/static/path/pattern", nil)
	if err != nil {
		b.Fatal(err)
	}

	benchRequest(b, router, req)
}

func Benchmark_5Routes(b *testing.B) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(Parameters(r).ByName("id")))
	}

	router := New(
		Route("/test/:id", handler),
		Route("/puff/path/:id", handler),
		Route("/home/user/:id", handler),
		Route("/home/jey/:id/:cat", handler),
		Route("/base/:id/user", handler),
	)

	req, err := http.NewRequest("GET", "http://localhost:8080/home/jey/5/user", nil)
	if err != nil {
		b.Fatal(err)
	}

	benchRequest(b, router, req)
}

type mockResponseWriter struct{}

func (m *mockResponseWriter) Header() (h http.Header) {
	return http.Header{}
}

func (m *mockResponseWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (m *mockResponseWriter) WriteString(s string) (n int, err error) {
	return len(s), nil
}

func (m *mockResponseWriter) WriteHeader(int) {}

func benchRequest(b *testing.B, router http.Handler, r *http.Request) {
	w := new(mockResponseWriter)
	u := r.URL
	rq := u.RawQuery
	r.RequestURI = u.RequestURI()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		u.RawQuery = rq
		router.ServeHTTP(w, r)
	}
}
