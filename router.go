package router

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Param is a single URL parameter, consisting of a key and a value.
type Param struct {
	Key, Value string
}

// Params is a Param-slice, as returned by the router.
// The slice is ordered, the first URL parameter is also the first slice value.
// It is therefore safe to read values by the index.
type Params []Param

// ByName returns the value of the first Param which key matches the given name.
// If no matching Param is found, an empty string is returned.
func (ps Params) ByName(name string) string {
	for i := range ps {
		if ps[i].Key == name {
			return ps[i].Value
		}
	}
	return ""
}

// Router is the robust interface allowing
// to compose dynamic levels of request matchers
// and all together implements http.Handler.
//
// Match func should return handler or nil
// if it cannot process the request.
type Router interface {
	http.Handler

	// Match should return nil if request
	// cannot be matched. At the top Router
	// nil could indicate that NotFound handler
	// can be applied.
	Match(*http.Request) http.Handler
}

// RouterFunc type is an adapter to allow the use of
// ordinary functions as Routers. If f is a function
// with the appropriate signature, RouterFunc(f) is a
// Router that calls f.
type RouterFunc func(*http.Request) http.Handler

// Match calls f(r).
func (rf RouterFunc) Match(r *http.Request) http.Handler {
	return rf(r)
}

// ServeHTTP calls f(w, r).
func (rf RouterFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h := rf(r); h != nil {
		h.ServeHTTP(w, r)
		if p := parameterized(r); p != nil {
			p.reset() // salvage request parameters
		}
	} else {
		http.NotFound(w, r)
	}
}

// New creates Router combined of given routes.
// It attempts to match all routes in order, the first
// matched route serves the request.
//
// Users may sort routes the way he prefers, or add
// dynamic sorting goroutine, which calculates order
// based on hits.
func New(routes ...Router) Router {
	return RouterFunc(func(r *http.Request) http.Handler {
		var found http.Handler
		for _, router := range routes {
			if found = router.Match(r); found != nil {
				break
			}
		}
		return found
	})
}

// Route creates Router which attempts
// to match given path to handler.
//
// Handler is a standard http.Handler which
// may be in the following formats:
//  http.Handler
//  http.HandlerFunc
//  func(http.ResponseWriter, *http.Request)
//
func Route(path string, handler interface{}) Router {
	p := "/" + strings.TrimLeft(path, "/")

	var h http.Handler = nil
	switch t := handler.(type) {
	case http.HandlerFunc:
		h = t
	case http.Handler:
		h = t
	case func(http.ResponseWriter, *http.Request):
		h = http.HandlerFunc(t)
	default:
		panic(fmt.Sprintf("not a handler given: %T - %+v", t, t))
	}

	// maybe static route
	if strings.IndexAny(p, ":*") == -1 {
		return RouterFunc(func(r *http.Request) http.Handler {
			if p == r.URL.Path {
				return h
			}
			return nil
		})
	}

	// first ensure dynamic pattern is valid
	var pos int
	for {
		if i := strings.IndexAny(p[pos:], ":*"); i == -1 {
			break
		} else {
			pos += i
		}

		switch {
		case p[pos-1] != '/':
			panic("special param matching signs, must follow after slash: " + p)
		case p[pos] == '*' && strings.IndexByte(p[pos:], '/') != -1:
			panic("match all sign, must be the last segment in pattern, without slash: " + p)
		case strings.IndexByte(p[pos:], '/') == pos+1:
			panic("parameter must be named: " + p)
		}
		pos++
	}

	// pool for parameters
	num := strings.Count(p, ":") + strings.Count(p, "*")
	pool := new(sync.Pool)
	pool.New = func() interface{} {
		return &parameters{all: make(Params, 0, num), pool: pool}
	}

	// dynamic route matcher
	return RouterFunc(func(r *http.Request) http.Handler {
		params := pool.Get().(*parameters)
		if match(p, r.URL.Path, &params.all) {
			params.wrap(r)
			return h
		}
		params.all = params.all[0:0]
		pool.Put(params)
		return nil
	})
}

func next(path string) int {
	if i := strings.IndexByte(path[1:], '/'); i != -1 {
		return i + 1
	}
	return len(path) // last path segment
}

func match(pat, url string, ps *Params) bool {
	if len(pat) <= 1 || len(url) <= 1 {
		return pat == url
	}

	i, j := next(pat), next(url)

	switch {
	case pat[1] == ':':
		n := len(*ps)
		*ps = (*ps)[:n+1]
		(*ps)[n].Key, (*ps)[n].Value = pat[2:i], url[1:j]
	case pat[1] == '*':
		n := len(*ps)
		*ps = (*ps)[:n+1]
		(*ps)[n].Key, (*ps)[n].Value = pat[2:i], url[1:len(url)]
		return true
	case pat[:i] != url[:j]:
		return false
	}

	return match(pat[i:], url[j:], ps)
}

// Files serves files from the given file system root.
// The path must end with "/*filepath", files are then served from the local
// path /defined/root/dir/*filepath.
//
// For example if root is "/etc" and *filepath is "passwd", the local file
// "/etc/passwd" would be served.
//
// Internally a http.FileServer is used, therefore http.NotFound is used instead
// of the Router's NotFound handler.
// To use the operating system's file system implementation,
// use http.Dir:
//     router.ServeFiles("/src/*filepath", http.Dir("/var/www"))
func Files(path string, root http.FileSystem) Router {
	if len(path) < 10 || path[len(path)-10:] != "/*filepath" {
		panic("path must end with /*filepath in path '" + path + "'")
	}

	files := http.FileServer(root)

	return Route(path, func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = Parameters(r).ByName("filepath")
		files.ServeHTTP(w, r)
	})
}

// Parameters returns all path parameters for given
// request.
//
// If there were no parameters and route is static
// then empty parameter slice is returned.
func Parameters(req *http.Request) Params {
	if p := parameterized(req); p != nil {
		return p.get()
	}
	return make(Params, 0)
}

// used to attach parameters to request
type paramReadCloser interface {
	io.ReadCloser
	get() Params
	reset()
}

type parameters struct {
	io.ReadCloser
	all  Params
	pool *sync.Pool
}

func (p *parameters) get() Params {
	return p.all
}

func (p *parameters) wrap(req *http.Request) {
	p.ReadCloser = req.Body
	req.Body = p
}

func (p *parameters) reset() {
	p.all = p.all[0:0]
	p.pool.Put(p)
}

func parameterized(req *http.Request) paramReadCloser {
	return req.Body.(paramReadCloser)
}
