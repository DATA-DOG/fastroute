// package mux is full featured http router
// using fastroute as core robust router, but enabling
// all usual features including:
//
//  - route optimization, for faster lookups
//  - trailing slash and fixed path redirects
//  - auto OPTIONS replies
//  - method not found handling
//
// This package is just an example for fastroute implementation,
// and will not be maintained to match all possible use
// cases. Instead you should copy and adapt it for certain
// customizations.
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
// Not found handler can be customized by extending the produced router with
// a middleware:
//
//  router := mux.New()
//  router.GET("/", Index)
//  router.GET("/hello/:name", Hello)
//
//  notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
//  	w.WriteHeader(404)
//  	fmt.Fprintln(w, "Ooops, looks like you mistyped the URL:", req.URL.Path)
//  })
//
//  server := router.Server()
//  log.Fatal(http.ListenAndServe(":8080", fastroute.RouterFunc(func(req *http.Request) http.Handler {
//  	if h := server.Match(req); h != nil {
//  		return h
//  	}
//  	return notFoundHandler
//  })))
//
// This in general says, that if there is no handler matched from registered
// routes, then use notFoundHandler
package mux

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/DATA-DOG/fastroute"
)

type route struct {
	path string
	h    http.Handler
}

// Mux request router
type Mux struct {
	// If enabled and none of routes match, then it
	// will try adding or removing trailing slash to
	// the path and redirect if there is such route.
	RedirectTrailingSlash bool

	// If enabled, attempts to fix path from multiple slashes
	// or dots.
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

	routes map[string][]*route
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
//
// Depending on ForceTrailingSlash, slash is either
// appended or removed at the end of the path.
func (m *Mux) Method(method, path string, handler interface{}) {
	if nil == m.routes {
		m.routes = make(map[string][]*route)
	}

	var h http.Handler = nil
	switch t := handler.(type) {
	case http.HandlerFunc:
		h = t
	case func(http.ResponseWriter, *http.Request):
		h = http.HandlerFunc(t)
	default:
		panic(fmt.Sprintf("not a handler given: %T - %+v", t, t))
	}

	method = strings.ToUpper(method)
	m.routes[method] = append(m.routes[method], &route{path, h})
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
	routes := m.optimize()

	router := fastroute.RouterFunc(func(req *http.Request) http.Handler {
		if router := routes[req.Method]; router != nil {
			if h := router.Match(req); h != nil {
				return h
			}
		}

		return nil
	})

	return fastroute.New(
		router, // maybe match configured routes
		m.redirectTrailingSlash(router),  // maybe trailing slash
		m.redirectFixedPath(router),      // maybe fix path
		m.autoOptions(routes),            // maybe options
		m.handleMethodNotAllowed(routes), // maybe not allowed method
	)
}

func (m *Mux) redirectTrailingSlash(router fastroute.Router) fastroute.Router {
	if !m.RedirectTrailingSlash {
		return router
	}

	return fastroute.RouterFunc(func(req *http.Request) http.Handler {
		p := req.URL.Path
		if p == "/" {
			return nil // nothing to fix
		}

		if p[len(p)-1] == '/' {
			p = p[:len(p)-1]
		} else {
			p += "/"
		}

		try, _ := http.NewRequest(req.Method, req.URL.String(), nil)
		try.URL.Path = p
		if h := router.Match(try); h != nil {
			fastroute.Recycle(try)
			return redirect(p)
		}
		return nil
	})
}

func (m *Mux) redirectFixedPath(router fastroute.Router) fastroute.Router {
	if !m.RedirectFixedPath {
		return router
	}
	return fastroute.RouterFunc(func(req *http.Request) http.Handler {
		try, _ := http.NewRequest(req.Method, req.URL.String(), nil)

		p := cleanPath(req.URL.Path)
		if p != req.URL.Path {
			try.URL.Path = p

			if h := router.Match(try); h != nil {
				fastroute.Recycle(try)
				return redirect(p)
			}
		}

		// now case insensitive match
		h := fastroute.ComparesPathWith(router, strings.EqualFold).Match(try)
		if h == nil {
			return nil
		}

		// matched case insensitive, lets fix the path
		pat := fastroute.Pattern(try)
		params := fastroute.Parameters(try)
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
		p = strings.Join(fixed, "/")
		fastroute.Recycle(try)

		return redirect(p)
	})
}

func (m *Mux) autoOptions(routers map[string]fastroute.Router) fastroute.Router {
	return fastroute.RouterFunc(func(req *http.Request) http.Handler {
		if req.Method != "OPTIONS" || !m.AutoOptionsReply {
			return nil
		}

		if allow := m.allowed(routers, req); len(allow) > 0 {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Allow", strings.Join(allow, ","))
			})
		}

		return nil
	})
}

func (m *Mux) handleMethodNotAllowed(routers map[string]fastroute.Router) fastroute.Router {
	return fastroute.RouterFunc(func(req *http.Request) http.Handler {
		if nil == m.MethodNotAllowed {
			return nil // not handled
		}

		if allow := m.allowed(routers, req); len(allow) > 0 {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Allow", strings.Join(allow, ","))
				m.MethodNotAllowed.ServeHTTP(w, r)
			})
		}

		return nil
	})
}

func (m *Mux) allowed(routers map[string]fastroute.Router, req *http.Request) []string {
	allow := make(map[string]bool)
	allow["OPTIONS"] = true
	for method, router := range routers {
		// Skip the requested method - we already tried this one
		if method == req.Method {
			continue
		}

		// server wide
		if req.URL.Path == "*" {
			allow[method] = true
			continue
		}

		// specific path
		if h := router.Match(req); h != nil {
			fastroute.Recycle(req)
			allow[method] = true
		}
	}

	var allows []string
	if len(allow) == 1 {
		return allows
	}

	for method := range allow {
		allows = append(allows, method)
	}
	return allows
}

func redirect(fixedPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.URL.Path = fixedPath
		http.Redirect(w, req, req.URL.String(), http.StatusPermanentRedirect)
	})
}

// this is just a way to optimize and combine
// routes to match them more efficiently
func (m *Mux) optimize() map[string]fastroute.Router {
	routes := make(map[string]fastroute.Router)

	for method, pack := range m.routes {
		static := make(map[string]http.Handler)
		var dynamic []fastroute.Router

		for _, route := range pack {
			if idx := strings.IndexAny(route.path, ":*"); idx == -1 {
				static[route.path] = route.h
			} else {
				dynamic = append(dynamic, fastroute.Route(route.path, route.h))
			}
		}

		var routers []fastroute.Router
		if len(static) > 0 {
			staticRouter := fastroute.RouterFunc(func(req *http.Request) http.Handler {
				return static[req.URL.Path]
			})
			routers = append(routers, staticRouter)
		}

		if len(dynamic) > 0 {
			routers = append(routers, fastroute.New(dynamic...))
		}

		routes[method] = fastroute.New(routers...)
	}
	return routes
}

// taken from https://github.com/julienschmidt/httprouter/blob/master/path.go
func cleanPath(p string) string {
	// Turn empty string into "/"
	if p == "" {
		return "/"
	}

	n := len(p)
	var buf []byte

	// Invariants:
	//      reading from path; r is index of next byte to process.
	//      writing to buf; w is index of next byte to write.

	// path must start with '/'
	r := 1
	w := 1

	if p[0] != '/' {
		r = 0
		buf = make([]byte, n+1)
		buf[0] = '/'
	}

	trailing := n > 2 && p[n-1] == '/'

	// A bit more clunky without a 'lazybuf' like the path package, but the loop
	// gets completely inlined (bufApp). So in contrast to the path package this
	// loop has no expensive function calls (except 1x make)

	for r < n {
		switch {
		case p[r] == '/':
			// empty path element, trailing slash is added after the end
			r++

		case p[r] == '.' && r+1 == n:
			trailing = true
			r++

		case p[r] == '.' && p[r+1] == '/':
			// . element
			r++

		case p[r] == '.' && p[r+1] == '.' && (r+2 == n || p[r+2] == '/'):
			// .. element: remove to last /
			r += 2

			if w > 1 {
				// can backtrack
				w--

				if buf == nil {
					for w > 1 && p[w] != '/' {
						w--
					}
				} else {
					for w > 1 && buf[w] != '/' {
						w--
					}
				}
			}

		default:
			// real path element.
			// add slash if needed
			if w > 1 {
				bufApp(&buf, p, w, '/')
				w++
			}

			// copy element
			for r < n && p[r] != '/' {
				bufApp(&buf, p, w, p[r])
				w++
				r++
			}
		}
	}

	// re-append trailing slash
	if trailing && w > 1 {
		bufApp(&buf, p, w, '/')
		w++
	}

	if buf == nil {
		return p[:w]
	}
	return string(buf[:w])
}

// internal helper to lazily create a buffer if necessary
func bufApp(buf *[]byte, s string, w int, c byte) {
	if *buf == nil {
		if s[w] == c {
			return
		}

		*buf = make([]byte, len(s))
		copy(*buf, s[:w])
	}
	(*buf)[w] = c
}
