// Package fastroute is static, composable high performance HTTP request router.
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
// interface as a building block together with path pattern matching in order
// to construct dynamic routes having named Params available from http.Request
// at zero allocation cost.
//
// Path parameters can be extracted from request this way:
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
//  Path: /:param
//
//  Requests:
//   /blog                               match: param="blog"
//   /                                   no match
//   /blog/go                            no match
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
//  Path: /*any
//
//  Requests:
//   /                                   match: any="/"
//   /files/dir                          match: any="/files/dir"
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
	if p, _ := req.Body.(*parameters); p != nil {
		return p.params
	}
	return nil
}

// Pattern gives matched route path pattern
// for this request if it has path parameters.
//
// If request parameters were already recycled,
// or route is static - it will return req.URL.Path.
func Pattern(req *http.Request) string {
	if p, _ := req.Body.(*parameters); p != nil {
		return p.pattern
	}
	return req.URL.Path // if matched will be same as url path
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
	if p, _ := req.Body.(*parameters); p != nil {
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

// Router interface extends http.Handler with one extra
// method - Route in order to route http.Request to http.Handler
// allowing to chain routes until one is matched.
//
// Route should route given request to
// the http.Handler. It may return nil if
// request cannot be handled. When ServeHTTP
// is invoked and handler is nil, it will
// serve http.NotFoundHandler
//
// Note, if the router is matched and it has
// path parameters - then it must be served
// in order to release allocated parameters
// back to the pool. Otherwise you will leak
// parameters, which you can also salvage by
// calling Recycle(http.Request)
type Router interface {
	http.Handler

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
		return RouterFunc(func(req *http.Request) http.Handler {
			if p == req.URL.Path {
				return h
			}
			return nil
		})
	}

	// prepare and validate pattern segments to match
	segments := strings.Split(strings.Trim(p, "/"), "/")
	for i, seg := range segments {
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
		if p, _ := req.Body.(*parameters); p != nil {
			p.reset(req)
		}
	})

	// dynamic route matcher
	return RouterFunc(func(req *http.Request) http.Handler {
		ps := pool.Get().(*parameters)
		if match(segments, req.URL.Path, &ps.params, ts) {
			ps.ReadCloser = req.Body
			req.Body = ps
			return handle
		}
		ps.params = ps.params[0:0]
		pool.Put(ps)
		return nil
	})
}

// matches pattern segments to an url and pushes named parameters to ps
func match(segments []string, url string, ps *Params, ts bool) bool {
	for _, segment := range segments {
		switch {
		case len(url) == 0 || url[0] != '/':
			return false
		case segment[1] == ':' && len(url) > 1:
			end := 1
			for end < len(url) && url[end] != '/' {
				end++
			}
			ps.push(segment[2:], url[1:end])
			url = url[end:]
		case segment[1] == '*':
			ps.push(segment[2:], url)
			return true
		case len(url) < len(segment) || url[:len(segment)] != segment:
			return false
		default:
			url = url[len(segment):]
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

func (p *parameters) reset(req *http.Request) {
	req.Body = p.ReadCloser
	p.params = p.params[0:0]
	p.pool.Put(p)
}
