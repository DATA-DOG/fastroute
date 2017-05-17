// package mux is full featured http router
// using fastroute as core robust router, but enabling
// all usual features like:
//
//  - request method based routes
//  - trailing slash and fixed path redirects
//  - auto OPTIONS replies
//  - method not found handling
//
// This package is just an example for fastroute implementation,
// and will not be maintained to match all possible use
// cases. Instead you should copy and adapt it for certain
// customizations if these are not sensible defaults.
//
// A trivial example is:
//
//  package main
//
//  import (
//      "fmt"
//      "log"
//      "net/http"
//      "github.com/DATA-DOG/fastroute/mux"
//  )
//
//  func Index(w http.ResponseWriter, r *http.Request) {
//      fmt.Fprint(w, "Welcome!\n")
//  }
//
//  func Hello(w http.ResponseWriter, r *http.Request) {
//      fmt.Fprintf(w, "hello, %s!\n", fastroute.Parameters(r).ByName("name"))
//  }
//
//  func main() {
//      router := mux.New()
//      router.GET("/", Index)
//      router.GET("/hello/:name", Hello)
//
//      log.Fatal(http.ListenAndServe(":8080", router.Server()))
//  }
//
// Not found handler can be customized by assigning custom handler:
//
//  router := mux.New()
//  router.GET("/", Index)
//  router.GET("/hello/:name", Hello)
//
//  router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
//  	w.WriteHeader(404)
//  	fmt.Fprintln(w, "Ooops, looks like you mistyped the URL:", req.URL.Path)
//  })
//
//  log.Fatal(http.ListenAndServe(":8080", router.Server()))
package mux

import (
	"net/http"
	"path"
	"strings"

	"github.com/DATA-DOG/fastroute"
)

// Mux request router
type Mux struct {
	// If enabled and none of routes match, then it
	// will try adding or removing trailing slash to
	// the path and redirect if there is such route.
	// If combined with RedirectFixedPath it may fix
	// both trailing slash and path.
	RedirectTrailingSlash bool

	// If enabled, attempts to fix path from multiple slashes
	// or dots also path case mismatches.
	// If combined with RedirectTrailingSlash it may fix
	// both trailing slash and path.
	RedirectFixedPath bool

	// If enabled, if request method is OPTIONS and there
	// is no route configured for this method. It will
	// respond with methods allowed for the matched path
	AutoOptionsReply bool

	// If set and requested path method is not allowed
	// but there are some of the methods allowed for this
	// path, then it will set allowed methods header and
	// use that handler to serve request, which usually should
	// respond with 405 status code
	MethodNotAllowed http.Handler

	// If set and request did not match any of routes,
	// then instead of default http.NotFoundHandler
	// it will use this.
	NotFound http.Handler

	routes map[string]fastroute.Router
}

// New creates Mux with default options
func New() *Mux {
	return &Mux{
		AutoOptionsReply:      true,
		RedirectFixedPath:     true,
		RedirectTrailingSlash: true,
	}
}

// Method registers handler for given request method
// and path.
func (m *Mux) Method(method, path string, handler interface{}) {
	m.Route(strings.ToUpper(method), fastroute.Route(path, handler))
}

// Method registers route for given request method.
// This might be useful when combining the routes or
// match them differently, like for example a
// map[string]http.Handler to match static routes
func (m *Mux) Route(method string, route fastroute.Router) {
	if nil == m.routes {
		m.routes = make(map[string]fastroute.Router)
	}

	if router, ok := m.routes[method]; ok {
		m.routes[method] = fastroute.New(router, route) // chain new route
	} else {
		m.routes[method] = route
	}
}

// GET is a shortcut for Method("GET", path, handler)
func (m *Mux) GET(path string, handler interface{}) {
	m.Method("GET", path, handler)
}

// HEAD is a shortcut for Method("HEAD", path, handler)
func (m *Mux) HEAD(path string, handler interface{}) {
	m.Method("HEAD", path, handler)
}

// OPTIONS is a shortcut for Method("OPTIONS", path, handler)
func (m *Mux) OPTIONS(path string, handler interface{}) {
	m.Method("OPTIONS", path, handler)
}

// POST is a shortcut for Method("POST", path, handler)
func (m *Mux) POST(path string, handler interface{}) {
	m.Method("POST", path, handler)
}

// PUT is a shortcut for Method("PUT", path, handler)
func (m *Mux) PUT(path string, handler interface{}) {
	m.Method("PUT", path, handler)
}

// PATCH is a shortcut for Method("PATCH", path, handler)
func (m *Mux) PATCH(path string, handler interface{}) {
	m.Method("PATCH", path, handler)
}

// DELETE is a shortcut for Method("DELETE", path, handler)
func (m *Mux) DELETE(path string, handler interface{}) {
	m.Method("DELETE", path, handler)
}

// Files server in order to serve files under given
// root directory, Path pattern must contain match all
// segment.
func (m *Mux) Files(path string, root http.FileSystem) {
	if pos := strings.IndexByte(path, '*'); pos == -1 {
		panic("path must end with match all: * segment'" + path + "'")
	} else {
		files := http.FileServer(root)
		m.GET(path, func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = fastroute.Parameters(r).ByName(path[pos+1:])
			files.ServeHTTP(w, r)
		})
	}
}

// Server compiles fastroute.Router aka http.Handler
// which is used as router for all registered routes,
//
// Routes are matched in following order:
//  1. static routes are matched from hashmap.
//  2. all routes having named parameters.
//
// If path does not match, not found handler is called,
// in order to customize it, wrap this resulted router.
func (m *Mux) Server() fastroute.Router {
	router := fastroute.RouterFunc(func(req *http.Request) http.Handler {
		if router := m.routes[req.Method]; router != nil {
			if h := router.Match(req); h != nil {
				return h
			}
		}

		return nil
	})

	return fastroute.New( // combines routers matched in given order
		router, // maybe match configured routes
		m.redirectTrailingOrFixedPath(router),          // maybe trailing slash or path fix
		fastroute.RouterFunc(m.autoOptions),            // maybe options
		fastroute.RouterFunc(m.handleMethodNotAllowed), // maybe not allowed method
		fastroute.RouterFunc(m.notFound),               // finally, custom or default not found handler
	)
}

func (m *Mux) notFound(req *http.Request) http.Handler {
	return m.NotFound // nil will result in fallback to default not found handler
}

func (m *Mux) redirectTrailingOrFixedPath(router fastroute.Router) fastroute.Router {
	if !m.RedirectFixedPath || !m.RedirectTrailingSlash {
		return router // nothing to try fixing
	}

	return fastroute.RouterFunc(func(req *http.Request) http.Handler {
		p := req.URL.Path
		rt := router
		if m.RedirectFixedPath {
			p = path.Clean(req.URL.Path)
			rt = fastroute.ComparesPathWith(router, strings.EqualFold) // case insensitive matching
		}

		attempts := []string{p}
		if m.RedirectTrailingSlash {
			if p[len(p)-1] == '/' {
				attempts = append(attempts, p[:len(p)-1]) // without trailing slash
			} else {
				attempts = append(attempts, p+"/") // with trailing slash
			}
		}

		try, _ := http.NewRequest(req.Method, "/", nil) // make request for all attempts
		for _, attempt := range attempts {
			try.URL.Path = attempt
			if h := rt.Match(try); h != nil {
				// matched, resolve fixed path and redirect
				pat, params := fastroute.Pattern(try), fastroute.Parameters(try)
				var fixed []string
				var nextParam int
				for _, segment := range strings.Split(pat, "/") {
					if strings.IndexAny(segment, ":*") != -1 {
						fixed = append(fixed, params[nextParam].Value)
						nextParam++
					} else {
						fixed = append(fixed, segment)
					}
				}
				defer fastroute.Recycle(try)
				return redirect(strings.Join(fixed, "/"))
			}
		}
		return nil // could not fix path
	})
}

func (m *Mux) autoOptions(req *http.Request) http.Handler {
	if req.Method != "OPTIONS" || !m.AutoOptionsReply {
		return nil
	}

	if allow := m.allowed(req); len(allow) > 0 {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", strings.Join(allow, ","))
		})
	}
	return nil
}

func (m *Mux) handleMethodNotAllowed(req *http.Request) http.Handler {
	if nil == m.MethodNotAllowed {
		return nil // not handled
	}

	if allow := m.allowed(req); len(allow) > 0 {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", strings.Join(allow, ","))
			m.MethodNotAllowed.ServeHTTP(w, r)
		})
	}
	return nil
}

func (m *Mux) allowed(req *http.Request) (allows []string) {
	added := make(map[string]bool)
	allow := func(m string) {
		if _, ok := added[m]; !ok {
			allows, added[m] = append(allows, m), true
		}
	}
	for method, router := range m.routes {
		// Skip the requested method - we already tried this one
		if method == req.Method {
			continue
		}

		// server wide
		if req.URL.Path == "*" {
			allow(method)
			continue
		}

		// specific path
		if h := router.Match(req); h != nil {
			fastroute.Recycle(req)
			allow(method)
		}
	}

	if len(allows) > 0 && m.AutoOptionsReply {
		allow("OPTIONS")
	}

	return
}

func redirect(fixedPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.URL.Path = fixedPath
		http.Redirect(w, req, req.URL.String(), http.StatusMovedPermanently)
	})
}
