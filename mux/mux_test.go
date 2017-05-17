package mux

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServeHTTP(t *testing.T) {
	routes := []string{
		"/a/:b/c",
		"/",
		"/a/b/",
		"/catch/*all",
		"/category/:cid/product/:pid",
		"/v/Äpfêl/",
	}
	handler := func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, req.URL.Path)
	}

	mux := New()
	mux.MethodNotAllowed = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-TESTED", "OK")
		w.WriteHeader(405)
		fmt.Fprintln(w, http.StatusText(405))
	})
	for _, path := range routes {
		mux.GET(path, handler)
	}
	mux.POST("/hello/:name", handler)

	mux.assertPatterns(t, []routerPattern{
		{"OPTIONS", "/a/b/", 200, map[string]string{"Allow": "GET,OPTIONS"}},                // allowed methods
		{"OPTIONS", "*", 200, map[string]string{"Allow": "GET,POST,OPTIONS"}},               // allowed methods
		{"GET", "/a/b", 301, map[string]string{"Location": "/a/b/"}},                        // has to be with trailing
		{"GET", "/a/b/", 200, map[string]string{}},                                          // exact match with trailing
		{"POST", "/a/b/", 405, map[string]string{"Allow": "GET,OPTIONS", "X-TESTED": "OK"}}, // method not allowed
		{"GET", "/a/bb/c/", 301, map[string]string{"Location": "/a/bb/c"}},                  // has to be without trailing
		{"GET", "/unknown", 404, map[string]string{}},                                       // simply not found
		{"GET", "/v/Äpfêl/", 200, map[string]string{}},                                      // normal unicode
		{"GET", "/v/äpfêL/", 301, map[string]string{"Location": "/v/%C3%84pf%C3%AAl/"}},     // redirect fixed path
		{"GET", "/v/äpfêL", 301, map[string]string{"Location": "/v/%C3%84pf%C3%AAl/"}},      // redirect fixed + trailing
	})

	// switch options
	mux.RedirectFixedPath = false
	mux.RedirectTrailingSlash = false
	mux.AutoOptionsReply = false

	mux.assertPatterns(t, []routerPattern{
		{"GET", "/a/b", 404, map[string]string{}},                    // has to be with trailing
		{"GET", "/a/bb/c/", 404, map[string]string{}},                // has to be without trailing
		{"GET", "/v/äpfêL", 404, map[string]string{}},                // has to be fixed
		{"OPTIONS", "/a/b/", 405, map[string]string{"Allow": "GET"}}, // method not allowed since disabled
	})

	mux.MethodNotAllowed = nil
	mux.NotFound = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-TESTED", "OK")
		w.WriteHeader(404)
		fmt.Fprintln(w, http.StatusText(404))
	})

	mux.assertPatterns(t, []routerPattern{
		{"GET", "/unknown", 404, map[string]string{"X-TESTED": "OK"}},  // custom not found
		{"OPTIONS", "/a/b/", 404, map[string]string{"X-TESTED": "OK"}}, // not allowed since disabled
	})
}

func TestFileServer(t *testing.T) {
	dir, err := ioutil.TempDir("", "router")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	tmpfn := filepath.Join(dir, "tmpfile")
	if err := ioutil.WriteFile(tmpfn, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	mux := New()
	mux.Files("/public/*files", http.Dir(dir))
	router := mux.Server()

	req, err := http.NewRequest("GET", "/public/tmpfile", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("unexpected response code: %d", w.Code)
	}

	if w.Body.String() != "hello world" {
		t.Fatalf("unexpected response body: %s", w.Body.String())
	}

	pattern := "/public/files"
	expectedMessage := "path must end with match all: * segment'/public/files'"
	defer func() {
		if err := recover(); err != nil {
			actual := fmt.Sprintf("%s", err)
			if actual != expectedMessage {
				t.Fatalf(`actual message: "%s" does not match expected: "%s"`, actual, expectedMessage)
			}
		}
	}()

	mux.Files(pattern, http.Dir(dir))

	t.Fatalf(`was expecting pattern: "%s" to panic with message: "%s"`, pattern, expectedMessage)
}

type routerPattern struct {
	method  string
	path    string
	code    int
	headers map[string]string
}

func (m *Mux) assertPatterns(t *testing.T, patterns []routerPattern) {
	for i, c := range patterns {
		req, _ := http.NewRequest(c.method, c.path, nil)
		w := httptest.NewRecorder()
		m.Server().ServeHTTP(w, req)

		if w.Code != c.code {
			t.Fatalf("expected status code: %d to match actual: %d at %d position", c.code, w.Code, i)
		}

		for k, v := range c.headers {
			if w.HeaderMap.Get(k) != v {
				t.Fatalf("expected header: %s at key: %s to match expected: %s at pos %d", w.HeaderMap.Get(k), k, v, i)
			}
		}
	}
}
