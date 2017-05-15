package mux

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/DATA-DOG/fastroute"
)

type route struct {
	path string
	h    http.Handler
}

// Mux request router
type Mux struct {
	// If enabled, all paths must end with a trailing slash
	// and when route is registered, it will append a slash
	// or redirect if it is missing.
	//
	// If disabled, it will remove trailing slash when path
	// is registered and redirect if matched against path
	// ending with trailing slash.
	ForceTrailingSlash bool
	RedirectFixedPath  bool
	AutoOptionsReply   bool

	MethodNotAllowed http.Handler
	NotFound         http.Handler

	routes map[string][]*route
}

// New creates Mux
func New() *Mux {
	return &Mux{}
}

// Method registers a route
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

	// ensure trailing slash as configured
	if len(path) > 1 {
		ts := path[len(path)-1] == '/'
		if m.ForceTrailingSlash && !ts {
			path += "/"
		}
		if !m.ForceTrailingSlash && ts {
			path = path[:len(path)-1]
		}
	}

	method = strings.ToUpper(method)
	m.routes[method] = append(m.routes[method], &route{path, h})
}

// Files server in order to serve files under given
// root directory, Path pattern must contain match all
// segment.
func (m *Mux) Files(path string, root http.FileSystem) {
	if pos := strings.IndexByte(path, '*'); pos == -1 {
		panic("path must end with match all: * segment'" + path + "'")
	} else {
		files := http.FileServer(root)
		m.Method("GET", path, func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = fastroute.Parameters(r).ByName(path[pos+1:])
			files.ServeHTTP(w, r)
		})
	}
}

// Server compiles http.Handler which is used as
// router for all registered routes,
//
// Routes are matched in following order:
//  1. static routes are matched from hashmap.
//  2. routes having static prefix are combined and matched one by one.
//  3. all remaining routes which are starting from dynamic segment.
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

	return router
}

func (m *Mux) redirectTrailingSlash(req *http.Request) http.Handler {
	p := req.URL.Path
	if p == "/" {
		return nil // nothing to fix
	}

	ts := p[len(p)-1] == '/'
	if m.ForceTrailingSlash && !ts {
		return redirect(p + "/")
	}

	if !m.ForceTrailingSlash && ts {
		return redirect(p[:len(p)-1])
	}

	return nil
}

func (m *Mux) redirectFixedPath(req *http.Request) http.Handler {
	p := req.URL.Path
	if p == "/" {
		return nil // nothing to fix
	}

	req2 := new(http.Request)
	*req2 = *req

	ts := p[len(p)-1] == '/'
	if m.ForceTrailingSlash && !ts {
		return redirect(p + "/")
	}

	if !m.ForceTrailingSlash && ts {
		return redirect(p[:len(p)-1])
	}

	return nil
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
		prefixed := make(map[string][]fastroute.Router)

		for _, route := range pack {
			if idx := strings.IndexAny(route.path, ":*"); idx == -1 {
				static[route.path] = route.h
			} else if idx > 1 {
				prefix := route.path[:idx]
				prefixed[prefix] = append(prefixed[prefix], fastroute.Route(route.path, route.h))
			} else {
				dynamic = append(dynamic, fastroute.Route(route.path, route.h))
			}
		}

		// @TODO: can be hit counting and resorting themselves
		var routers []fastroute.Router
		if len(static) > 0 {
			staticRouter := fastroute.RouterFunc(func(req *http.Request) http.Handler {
				return static[req.URL.Path]
			})
			routers = append(routers, staticRouter)
		}

		prefixedSquashed := make([]searchPrefixed, len(prefixed))
		for static, routers := range prefixed {
			prefixedSquashed = append(prefixedSquashed, searchPrefixed{static, fastroute.New(routers...)})
		}
		sort.Sort(byLength(prefixedSquashed))

		if len(prefixedSquashed) > 0 {
			prefixedRouter := fastroute.RouterFunc(func(req *http.Request) http.Handler {
				path := req.URL.Path
				for _, prefixed := range prefixedSquashed {
					s := prefixed.prefix
					if len(path) > len(s) && path[:len(s)] == s {
						return prefixed.router.Match(req)
					}
				}
				return nil
			})
			routers = append(routers, prefixedRouter)
		}

		if len(dynamic) > 0 {
			routers = append(routers, fastroute.New(dynamic...))
		}

		routes[method] = fastroute.New(routers...)
	}
	return routes
}

type searchPrefixed struct {
	prefix string
	router fastroute.Router
}

type byLength []searchPrefixed

func (s byLength) Len() int      { return len(s) }
func (s byLength) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byLength) Less(i, j int) bool {
	a, b := len(s[i].prefix), len(s[j].prefix)
	if a == b {
		return s[i].prefix < s[j].prefix
	}
	return a > b
}
