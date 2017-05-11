package router

import (
	"net/http"
	"net/http/httptest"
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

func BenchmarkDynamic(b *testing.B) {
	router := Path("/v1/users/:id", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Context().Value(Params).(Parameters).ByName("id")))
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

// func BenchmarkParams(b *testing.B) {
// 	ps := make(Parameters, 1)
// 	b.ReportAllocs()
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		// ps = ps[:cap(ps)]
// 		ps[0].Key = "a"
// 		ps[0].Value = "v"
// 	}
// }

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
	var request *http.Request
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
	})
	routes := []Router{
		Get("/a/:b/c", handler),
		Get("/category/:cid/product/*rest", handler),
		Get("/users/:id/:bid/", handler),
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

		if h == nil {
			continue
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		for key, val := range c.params {
			act := request.Context().Value(Params)
			if v, ok := act.(Parameters); ok {
				if v.ByName(key) != val {
					t.Fatalf("param: %s expected %s does not match to: %s, case: %d", key, val, v, i)
				}
			} else {
				t.Fatalf("could not locate param: %s, case: %d, val: %+v", key, i, act)
			}
		}
	}
}
