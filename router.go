// Package fastroute is http.Handler based high performance HTTP request router.
//
// A trivial example is:
//
//  package main
//
//  import (
//      "fmt"
//      "log"
//      "net/http"
//      "github.com/DATA-DOG/fastroute"
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
//      log.Fatal(http.ListenAndServe(":8080", fastroute.New(
//          fastroute.Route("/", Index),
//          fastroute.Route("/hello/:name", Hello),
//      )))
//  }
//
// The router can be composed of fastroute.Router interface, which shares
// the same htto.Handler interface. This package provides only this orthogonal
// interface as a building block.
//
// It also provides path pattern matching in order to construct dynamic routes
// having path Params available from http.Request at zero allocation cost.
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
//   /blog/go/request-routers/           no match, but the router would redirect
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
//   /files                              no match, but the router would redirect
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
	return make(Params, 0)
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
	//
	// Note, if the router is matched and it has
	// path parameters - then it must be served
	// in order to release allocated parameters
	// back to the pool. Otherwise you might
	// introduce memory leaks
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
// If the route path is static (does not contain parameters)
// then it will be matched as is to an URL.
//
// Otherwise if path contains any parameters, it then
// will load parameters from sync.Pool which scales based
// on load you have. Attempts to match route.
func Route(path string, handler interface{}) Router {
	p := "/" + strings.TrimLeft(path, "/")

	var h http.Handler = nil
	switch t := handler.(type) {
	case http.HandlerFunc:
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
	pool := sync.Pool{New: func() interface{} {
		return &parameters{params: make(Params, 0, num)}
	}}

	// extend handler in order to salvage parameters
	handle := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		if p := parameterized(r); p != nil {
			p.params = p.params[0:0]
			pool.Put(p)
		}
	})

	// dynamic route matcher
	return RouterFunc(func(r *http.Request) http.Handler {
		p := pool.Get().(*parameters)
		if match(segments, r.URL.Path, &p.params, ts) {
			p.wrap(r)
			return handle
		}
		p.params = p.params[0:0]
		pool.Put(p)
		return nil
	})
}

func match(segments []string, url string, ps *Params, ts bool) bool {
	for _, seg := range segments {
		lu := len(url)
		switch {
		case lu == 0:
			return false
		case seg[1] == ':': // match param
			n := len(*ps)
			*ps = (*ps)[:n+1]
			end := 1
			for end < lu && url[end] != '/' {
				end++
			}

			(*ps)[n].Key, (*ps)[n].Value = seg[2:], url[1:end]
			url = url[end:]
		case seg[1] == '*': // match remaining
			n := len(*ps)
			*ps = (*ps)[:n+1]
			(*ps)[n].Key, (*ps)[n].Value = seg[2:], url[1:]
			return true
		case lu < len(seg): // ensure length
			return false
		case url[:len(seg)] == seg: // match static
			url = url[len(seg):]
		default:
			return false
		}
	}
	return (!ts && url == "") || (ts && url == "/") // match trailing slash
}

type parameters struct {
	io.ReadCloser
	params Params
}

func (p *parameters) wrap(req *http.Request) {
	p.ReadCloser = req.Body
	req.Body = p
}

func parameterized(req *http.Request) *parameters {
	if p, ok := req.Body.(*parameters); ok {
		return p
	}
	return nil
}
