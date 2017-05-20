// Package fastroute is standard http.Handler based high performance HTTP request router.
//
// A trivial example is:
//
//  package main
//
//  import (
//      "fmt"
//      "net/http"
//
//      fr "github.com/DATA-DOG/fastroute"
//  )
//
//  var routes = map[string]fr.Router{
//      "GET": fr.Chain(
//          fr.New("/", handler),
//          fr.New("/hello/:name/:surname", handler),
//          fr.New("/hello/:name", handler),
//      ),
//      "POST": fr.Chain(
//          fr.New("/users", handler),
//          fr.New("/users/:id", handler),
//      ),
//  }
//
//  var router = fr.RouterFunc(func(req *http.Request) http.Handler {
//      return routes[req.Method] // fastroute.Router is also http.Handler
//  })
//
//  func main() {
//      http.ListenAndServe(":8080", router)
//  }
//
//  func handler(w http.ResponseWriter, req *http.Request) {
//      fmt.Fprintln(w, fmt.Sprintf(
//          `%s "%s", pattern: "%s", parameters: "%v"`,
//          req.Method,
//          req.URL.Path,
//          fr.Pattern(req),
//          fr.Parameters(req),
//      ))
//  }
//
// The router can be composed of fastroute.Router interface, which shares
// the same http.Handler interface. This package provides only this orthogonal
// interface as a building block.
//
// It also provides path pattern matching in order to construct dynamic routes
// having named Params available from http.Request at zero allocation cost.
// You can extract path parameters from request this way:
//
//  params := fastroute.Parameters(request) // request - *http.Request
//  fmt.Println(params.ByName("id"))
//
// The registered path, against which the router matches incoming requests, can
// contain two types of parameters:
//  Syntax    Type
//  :name     named parameter
//  *name     catch-all parameter
//
// Named parameters are dynamic path segments. They match anything until the
// next '/' or the path end:
//  Path: /blog/:category/:post
//
//  Requests:
//   /blog/go/request-routers            match: category="go", post="request-routers"
//   /blog/go/request-routers/           no match
//   /blog/go/                           no match
//   /blog/go/request-routers/comments   no match
//
// Catch-all parameters match anything until the path end, including the
// directory index (the '/' before the catch-all). Since they match anything
// until the end, catch-all parameters must always be the final path element.
//  Path: /files/*filepath
//
//  Requests:
//   /files/                             match: filepath="/"
//   /files/LICENSE                      match: filepath="/LICENSE"
//   /files/templates/article.html       match: filepath="/templates/article.html"
//   /files                              no match
//
package fastroute

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Parameters returns all path parameters for given
// request.
//
// If there were no parameters and route is static
// then empty parameter slice is returned.
func Parameters(req *http.Request) Params {
	if p := parameterized(req); p != nil {
		return p.params
	}
	return emptyParams
}

// Pattern gives matched route path pattern
// for this request.
//
// If request parameters were already flushed,
// meaning - it was either served or recycled
// manually, then empty string will be returned.
func Pattern(req *http.Request) string {
	if p := parameterized(req); p != nil {
		return p.pattern
	}
	return ""
}

// Recycle resets named parameters
// if they were assigned to the request.
//
// When using Router.Route(http.Request) func,
// parameters will be flushed only if matched
// http.Handler is served.
//
// If the purpose is just to test Router
// whether it matches or not, without serving
// matched handler, then this method should
// be invoked to prevent leaking parameters.
//
// If the route is not matched and handler is nil,
// then parameters will not be allocated, same
// as for static paths.
func Recycle(req *http.Request) {
	if p := parameterized(req); p != nil {
		p.reset(req)
	}
}

// Params is a slice of key value pairs, as extracted from
// the http.Request served by Router.
//
// The slice is ordered, the first URL parameter is also the first slice value.
// It is therefore safe to read values by the index.
type Params []struct{ Key, Value string }

// ByName returns the value of the first Param which key matches the given name.
// If no matching param is found, an empty string is returned.
func (ps Params) ByName(name string) string {
	for i := range ps {
		if ps[i].Key == name {
			return ps[i].Value
		}
	}
	return ""
}

// used internally to lazily append parameters
func (ps *Params) push(key, val string) {
	n := len(*ps)
	*ps = (*ps)[:n+1]
	(*ps)[n].Key, (*ps)[n].Value = key, val
}

// Router interface is robust and nothing more than
// http.Handler. It simply extends it with one extra method -
// Route in order to route http.Request to http.Handler.
// This way allows to chain it until a handler is matched.
//
// Route func should return handler or nil.
type Router interface {
	http.Handler

	// Route should route given request to
	// the http.Handler. It may return nil if
	// request cannot be matched. When ServeHTTP
	// is invoked and handler is nil, it will
	// serve http.NotFoundHandler
	//
	// Note, if the router is matched and it has
	// path parameters - then it must be served
	// in order to release allocated parameters
	// back to the pool. Otherwise you will leak
	// parameters, which you can also salvage by
	// calling Recycle on http.Request
	Route(*http.Request) http.Handler
}

// RouterFunc type is an adapter to allow the use of
// ordinary functions as Routers. If f is a function
// with the appropriate signature, RouterFunc(f) is a
// Router that calls f.
type RouterFunc func(*http.Request) http.Handler

// Route calls f(req) to return http.Handler.
// In case if it was Router, delegates that call
// to re-route the http.Request
func (f RouterFunc) Route(req *http.Request) http.Handler {
	h := f(req)
	if r, ok := h.(Router); ok {
		return r.Route(req)
	}
	return h
}

// ServeHTTP calls f(req) to get http.Handler and serve it,
// or fallback to http.NotFound.
func (f RouterFunc) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if h := f(req); h != nil {
		h.ServeHTTP(w, req)
	} else {
		http.NotFound(w, req)
	}
}

// Chain routes into single Router. Tries all given
// routes in order, until the first one, which is
// able to Route the request.
//
// Users may sort routes on their preference, or even
// add hit counting sorting goroutine, which calculates order
// based on hits.
func Chain(routes ...Router) Router {
	return RouterFunc(func(req *http.Request) http.Handler {
		for _, router := range routes {
			if handler := router.Route(req); handler != nil {
				return handler
			}
		}
		return nil
	})
}

// New creates Router which attempts
// to route the request by matching path.
//
// Handler is a standard http.Handler which
// may be accepted in the following formats:
//  http.Handler
//  func(http.ResponseWriter, *http.Request)
//
// Static paths will be simply compared with
// requested path. While paths having named
// parameters will be matched by each path segment.
// And bind named parameters to http.Request.
//
// When the request is routed, it must be served
// or recycled in order to salvage allocated named
// parameters back to the sync.Pool, which dynamically
// expands or shrinks based on concurrency.
func New(path string, handler interface{}) Router {
	p := "/" + strings.TrimLeft(path, "/")

	var h http.Handler = nil
	switch t := handler.(type) {
	case http.HandlerFunc:
		h = t
	case func(http.ResponseWriter, *http.Request):
		h = http.HandlerFunc(t)
	case nil:
		panic("given handler cannot be: nil")
	default:
		panic(fmt.Sprintf("not a handler given: %T - %+v", t, t))
	}

	// maybe static route
	if strings.IndexAny(p, ":*") == -1 {
		ps := &parameters{params: emptyParams, pattern: p}
		return RouterFunc(func(req *http.Request) http.Handler {
			if p == req.URL.Path {
				ps.wrap(req)
				return h
			}
			return nil
		})
	}

	// prepare and validate pattern segments to match
	segments := strings.Split(strings.Trim(p, "/"), "/")
	for i := 0; i < len(segments); i++ {
		seg := segments[i]
		segments[i] = "/" + seg
		if pos := strings.IndexAny(seg, ":*"); pos == -1 {
			continue
		} else if pos != 0 {
			panic("special param matching signs, must follow after slash: " + p)
		} else if len(seg)-1 == pos {
			panic("param must be named after sign: " + p)
		} else if seg[0] == '*' && i+1 != len(segments) {
			panic("match all, must be the last segment in pattern: " + p)
		} else if strings.IndexAny(seg[1:], ":*") != -1 {
			panic("only one param per segment: " + p)
		}
	}
	ts := p[len(p)-1] == '/' // whether we need to match trailing slash

	// pool for parameters
	num := strings.Count(p, ":") + strings.Count(p, "*")
	pool := sync.Pool{}
	pool.New = func() interface{} {
		return &parameters{params: make(Params, 0, num), pool: &pool, pattern: p}
	}

	// extend handler in order to salvage parameters
	handle := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h.ServeHTTP(w, req)
		if p := parameterized(req); p != nil {
			p.reset(req)
		}
	})

	// dynamic route matcher
	return RouterFunc(func(req *http.Request) http.Handler {
		p := pool.Get().(*parameters)
		if match(segments, req.URL.Path, &p.params, ts) {
			p.wrap(req)
			return handle
		}
		p.reset(req)
		return nil
	})
}

// matches pattern segments to an url and pushes named parameters to ps
func match(segments []string, url string, ps *Params, ts bool) bool {
	for _, segment := range segments {
		if len(url) == 0 {
			return false
		} else if segment[1] == ':' {
			end := 1
			for end < len(url) && url[end] != '/' {
				end++
			}
			ps.push(segment[2:], url[1:end])
			url = url[end:]
		} else if segment[1] == '*' {
			ps.push(segment[2:], url)
			return true
		} else if len(url) < len(segment) {
			return false
		} else if url[:len(segment)] == segment {
			url = url[len(segment):]
		} else {
			return false
		}
	}
	return (!ts && url == "") || (ts && url == "/") // match trailing slash
}

type parameters struct {
	io.ReadCloser
	params  Params
	pattern string
	pool    *sync.Pool
}

func (p *parameters) wrap(req *http.Request) {
	p.ReadCloser = req.Body
	req.Body = p
}

func (p *parameters) reset(req *http.Request) {
	if p.pool != nil { // only routes with path parameters have a pool
		p.params = p.params[0:0]
		p.pool.Put(p)
	}
	req.Body = p.ReadCloser
}

func parameterized(req *http.Request) *parameters {
	if p, ok := req.Body.(*parameters); ok {
		return p
	}
	return nil
}

var emptyParams = make(Params, 0, 0)
