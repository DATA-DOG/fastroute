package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
)

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

func BenchmarkHttpRouterParam(b *testing.B) {
	router := httprouter.New()
	router.GET("/v1/users/:id", func(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
		w.Write([]byte(ps.ByName("id")))
	})

	req, err := http.NewRequest("GET", "http://localhost:8080/v1/users/5", nil)
	if err != nil {
		b.Fatal(err)
	}
	w := &mockResponseWriter{}
	router.ServeHTTP(w, req) // warmup

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouterParam(b *testing.B) {
	router := Route("/v1/users/:id", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(Parameters(r).ByName("id")))
	})

	req, err := http.NewRequest("GET", "http://localhost:8080/v1/users/5", nil)
	if err != nil {
		b.Fatal(err)
	}
	w := &mockResponseWriter{}
	router.ServeHTTP(w, req) // warmup

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter5Routes(b *testing.B) {
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
	w := &mockResponseWriter{}
	router.ServeHTTP(w, req) // warmup

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkHttpRouter5Routes(b *testing.B) {
	router := httprouter.New()
	handler := func(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
		w.Write([]byte(ps.ByName("id")))
	}
	router.GET("/test/:id", handler)
	router.GET("/puff/path/:id", handler)
	router.GET("/home/user/:id", handler)
	router.GET("/home/jey/:id/:cat", handler)
	router.GET("/base/:id/user", handler)

	req, err := http.NewRequest("GET", "http://localhost:8080/home/jey/5/user", nil)
	if err != nil {
		b.Fatal(err)
	}
	w := &mockResponseWriter{}
	router.ServeHTTP(w, req) // warmup

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

func TestStaticRouteMatcher(t *testing.T) {
	cases := map[string]bool{
		"/users/hello":  true,
		"/user/hello":   false,
		"/users/hello/": false,
	}
	router := Route("/users/hello", http.NotFoundHandler())

	for p, b := range cases {
		req, err := http.NewRequest("GET", p, nil)
		if err != nil {
			t.Fatal(err)
		}
		if b && router.Match(req) == nil {
			t.Fatalf("expected to match: %s", p)
		}
		if !b && router.Match(req) != nil {
			t.Fatalf("did not expect to match: %s", p)
		}
	}
}

func TestDynamicRouteMatcher(t *testing.T) {
	var request *http.Request
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
	})
	routes := []Router{
		Route("/a/:b/c", handler),
		Route("/category/:cid/product/*rest", handler),
		Route("/users/:id/:bid/", handler),
	}

	router := New(routes...)

	cases := []struct {
		path   string
		params map[string]string
		match  bool
	}{
		{"/a/dic/c", map[string]string{"b": "dic"}, true},
		{"/a/d/c", map[string]string{"b": "d"}, true},
		{"/a/c", map[string]string{}, false},
		{"/a/c/c", map[string]string{"b": "c"}, true},
		{"/a/c/b", map[string]string{}, false},
		{"/a/c/c/", map[string]string{}, false},
		{"/category/5/product/x/a/bc", map[string]string{"cid": "5", "rest": "x/a/bc"}, true},
		{"/users/a/b/", map[string]string{"id": "a", "bid": "b"}, true},
		{"/users/a/b/be/", map[string]string{}, false},
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

		if h == nil {
			continue
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		for key, val := range c.params {
			act := Parameters(req).ByName(key)
			if act != val {
				t.Fatalf("param: %s expected %s does not match to: %s, case: %d", key, val, act, i)
			}
		}
	}
}
