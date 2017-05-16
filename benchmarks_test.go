package fastroute

import (
	"net/http"
	"testing"
)

func Benchmark_1Param(b *testing.B) {
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
	w := &mockResponseWriter{}
	router.ServeHTTP(w, req) // warmup

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

func Benchmark_HandlesRoute(b *testing.B) {
	router := Route("/test/:id", http.NotFoundHandler())

	req, err := http.NewRequest("GET", "http://localhost:8080/test/5", nil)
	if err != nil {
		b.Fatal(err)
	}

	if ok, params := Handles(router, req); !ok {
		b.Fatal("should have matched")
	} else if params.ByName("id") != "5" {
		b.Fatal("should have matched param")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Handles(router, req)
	}
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
