package router

import (
	"net/http"
	"testing"
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

func BenchmarkStd(b *testing.B) {
	router := Get("/v1/users/:id", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Context().Value("id").(string)))
	}))

	req, err := http.NewRequest("GET", "http://localhost:8080/v1/users/5", nil)
	if err != nil {
		b.Fatal(err)
	}
	w := &mockResponseWriter{}

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
	router := Get("/users/hello", http.NotFoundHandler())

	for p, b := range cases {
		req, err := http.NewRequest("GET", p, nil)
		if err != nil {
			t.Fatal(err)
		}
		if b && router.Route(req) == nil {
			t.Fatalf("expected to match: %s", p)
		}
		if !b && router.Route(req) != nil {
			t.Fatalf("did not expect to match: %s", p)
		}
	}
}

func TestDynamicRouteMatcher(t *testing.T) {
	routes := []Router{
		Get("/a/:b/c", http.NotFoundHandler()),
		Get("/category/:cid/product/*rest", http.NotFoundHandler()),
		Get("/users/:id/:bid/", http.NotFoundHandler()),
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
	}

	for _, c := range cases {
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
	}
}
