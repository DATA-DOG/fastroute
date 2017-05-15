package fastroute

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
	handler := http.NotFoundHandler()
	router := New(
		Route("/a/:b/c", handler),
		Route("/category/:cid/product/*rest", handler),
		Route("/users/:id/:bid/", handler),
		Route("/applications/:client_id/tokens", handler),
		Route("/repos/:owner/:repo/issues/:number/labels/:name", handler),
		Route("/files/*filepath", handler),
	)

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
		{"/category/5/product/x/a/bc", map[string]string{"cid": "5", "rest": "/x/a/bc"}, true},
		{"/users/a/b/", map[string]string{"id": "a", "bid": "b"}, true},
		{"/users/a/b/be/", map[string]string{}, false},
		{"/applications/:client_id/tokens", map[string]string{"client_id": ":client_id"}, true},
		{"/repos/:owner/:repo/issues/:number/labels", map[string]string{}, false},
		{"/files/LICENSE", map[string]string{"filepath": "/LICENSE"}, true},
		{"/files/", map[string]string{"filepath": "/"}, true},
		{"/files", map[string]string{}, false},
		{"/files/css/style.css", map[string]string{"filepath": "/css/style.css"}, true},
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

		for key, val := range c.params {
			act := Parameters(req).ByName(key)
			if act != val {
				t.Fatalf("param: %s expected %s does not match to: %s, case: %d", key, val, act, i)
			}
		}

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

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
