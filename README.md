[![Build Status](https://travis-ci.org/DATA-DOG/fastroute.svg?branch=master)](https://travis-ci.org/DATA-DOG/fastroute)
[![GoDoc](https://godoc.org/github.com/DATA-DOG/fastroute?status.svg)](https://godoc.org/github.com/DATA-DOG/fastroute)
[![codecov.io](https://codecov.io/github/DATA-DOG/fastroute/branch/master/graph/badge.svg)](https://codecov.io/github/DATA-DOG/fastroute)

# FastRoute

Insanely **simple**, **idiomatic** and **fast** - **161** loc
http router for `golang`. Uses standard **http.Handler** and
has no limitations to path matching compared to routers
derived from **Trie (radix)** tree based solutions.

> Less is exponentially more

**fastroute.Router** interface extends **http.Handler** with one extra
method - **Route** in order to route **http.Request** to **http.Handler**
allowing to chain routes until one is matched.

> Go is about composition

The gravest problem all routers have - is the central structure
holding all the context.

**fastroute** is extremely flexible, because it has only static,
unbounded functions. Allows unlimited ways to compose router.
The exported API is done and will never change, **backward
compatibility is now guaranteed**.

See the following example:

``` go
package main

import (
	"fmt"
	"net/http"

	fr "github.com/DATA-DOG/fastroute"
)

var routes = map[string]fr.Router{
	"GET": fr.Chain(
		fr.New("/", handler),
		fr.New("/hello/:name/:surname", handler),
		fr.New("/hello/:name", handler),
	),
	"POST": fr.Chain(
		fr.New("/users", handler),
		fr.New("/users/:id", handler),
	),
}

var router = fr.RouterFunc(func(req *http.Request) http.Handler {
	return routes[req.Method] // fastroute.Router is also http.Handler
})

func main() {
	http.ListenAndServe(":8080", router)
}

func handler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(w, fmt.Sprintf(
		`%s "%s", pattern: "%s", parameters: "%v"`,
		req.Method,
		req.URL.Path,
		fr.Pattern(req),
		fr.Parameters(req),
	))
}
```

In overall, it is **not all in one** router, it is the same **http.Handler**
with do it yourself style, but with **zero allocations** path pattern matching.
Feel free to just copy it and adapt to your needs.

It deserves a [quote](http://users.ece.utexas.edu/~adnan/pike.html) from **Rob Pike**:

> Fancy algorithms are slow when n is small, and n is usually small. Fancy
> algorithms have big constants. Until you know that n is frequently going
> to be big, don't get fancy.

The trade off this router makes is the size of **n**. Instead it provides
orthogonal building blocks, just like **http.Handler** does, in order to build
customized routers.

See [benchmark results](#benchmarks) for more details.

## Guides

Here are some common usage guidelines:

- [Custom Not Found](#custom-not-found-handler)
- [Method Not Found](#method-not-found-support)
- [Options](#options)
- [Combining static routes](#combining-static-routes)
- [Trailing slash or fixed path redirects](#trailing-slash-or-fixed-path-redirects)
- [Named routes](#named-routes)
- [Hit counting frequently accessed routes](#hit-counting-frequently-accessed-routes)

### Custom Not Found handler

Since **fastroute.Router** returns **nil** if request is not matched, we can easily
extend it and create middleware for it at as many levels as we like.

``` go
package main

import (
	"fmt"
	"net/http"

	"github.com/DATA-DOG/fastroute"
)

func main() {
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(404)
		fmt.Fprintln(w, "Ooops, looks like you mistyped the URL:", req.URL.Path)
	})

	router := fastroute.New("/users/:id", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintln(w, "user:", fastroute.Parameters(req).ByName("id"))
	})

	http.ListenAndServe(":8080", fastroute.RouterFunc(func(req *http.Request) http.Handler {
		if h := router.Route(req); h != nil {
			return h
		}
		return notFoundHandler
	}))
}
```

This way, it is possible to extend **fastroute.Router** with various middleware, including:
- Method not found handler.
- Fixed path or trailing slash redirects. Based on your chosen route layout.
- Options or **CORS**.

### Method not found support

**Fastroute** provides way to check whether request can be served, not only
serve it. Though, the parameters then must be recycled in order to prevent
leaking. When a routed request is served, it automatically recycles.

``` go
package main

import (
	"fmt"
	"net/http"
	"strings"

	fr "github.com/DATA-DOG/fastroute"
)

var routes = map[string]fr.Router{
	"GET":    fr.New("/users", handler),
	"POST":   fr.New("/users/:id", handler),
	"PUT":    fr.New("/users/:id", handler),
	"DELETE": fr.New("/users/:id", handler),
}

var router = fr.RouterFunc(func(req *http.Request) http.Handler {
	return routes[req.Method] // fastroute.Router is also http.Handler
})

var app = fr.RouterFunc(func(req *http.Request) http.Handler {
	if h := router.Route(req); h != nil {
		return h // routed and can be served
	}

	var allows []string
	for method, routes := range routes {
		if h := routes.Route(req); h != nil {
			allows = append(allows, method)
			fr.Recycle(req) // we will not serve it, need to recycle
		}
	}

	if len(allows) == 0 {
		return nil
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Allow", strings.Join(allows, ","))
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintln(w, http.StatusText(http.StatusMethodNotAllowed))
	})
})

func main() {
	http.ListenAndServe(":8080", app)
}

func handler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(w, fmt.Sprintf(
		`%s "%s", pattern: "%s", parameters: "%v"`,
		req.Method,
		req.URL.Path,
		fr.Pattern(req),
		fr.Parameters(req),
	))
}
```

If we make a request: `curl -i http://localhost:8080/users/1`, we will get:

```
HTTP/1.1 405 Method Not Allowed
Allow: PUT,DELETE,POST
Date: Fri, 19 May 2017 06:09:56 GMT
Content-Length: 19
Content-Type: text/plain; charset=utf-8

Method Not Allowed
```

### Options

Middleware example for **OPTIONS**:

``` go
package main

import (
	"fmt"
	"net/http"
	"strings"

	fr "github.com/DATA-DOG/fastroute"
)

var routes = map[string]fr.Router{
	"GET":    fr.New("/users", handler),
	"POST":   fr.New("/users/:id", handler),
	"PUT":    fr.New("/users/:id", handler),
	"DELETE": fr.New("/users/:id", handler),
}

var router = fr.RouterFunc(func(req *http.Request) http.Handler {
	return routes[req.Method] // fastroute.Router is also http.Handler
})

func main() {
	http.ListenAndServe(":8080", fr.Chain(
		router,          // maybe one of routes
		options(routes), // fallback to options if requested
		// maybe method not allowed
		// maybe redirect fixed path
		// not found then
	))
}

func handler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(w, fmt.Sprintf(
		`%s "%s", pattern: "%s", parameters: "%v"`,
		req.Method,
		req.URL.Path,
		fr.Pattern(req),
		fr.Parameters(req),
	))
}

func options(routes map[string]fr.Router) fr.Router {
	return fr.RouterFunc(func(req *http.Request) http.Handler {
		if req.Method != "OPTIONS" {
			return nil
		}

		fmt.Println(req.URL.Path)
		var allows []string
		for method, routes := range routes {
			if req.URL.Path == "*" {
				// though most of the tools like curl, does not support such a request
				allows = append(allows, method)
				continue
			}

			if h := routes.Route(req); h != nil {
				allows = append(allows, method)
				fr.Recycle(req) // we will not serve it, need to recycle
			}
		}

		if len(allows) == 0 {
			return nil
		}

		allows = append(allows, "OPTIONS")

		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Allow", strings.Join(allows, ","))
		})
	})
}
```

If we make a request: `curl -i -X OPTIONS http://localhost:8080/users/1`, we will get:

```
HTTP/1.1 200 OK
Allow: POST,PUT,DELETE,OPTIONS
Date: Tue, 23 May 2017 07:31:47 GMT
Content-Length: 0
Content-Type: text/plain; charset=utf-8
```

### Combining static routes

The best and fastest way to match static routes - is to have a **map** of path -> handler pairs.

``` go
package main

import (
	"fmt"
	"net/http"

	"github.com/DATA-DOG/fastroute"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintln(w, req.URL.Path, fastroute.Parameters(req))
	})

	static := map[string]http.Handler{
		"/status":      handler,
		"/users/roles": handler,
	}

	staticRoutes := fastroute.RouterFunc(func(req *http.Request) http.Handler {
		return static[req.URL.Path]
	})

	dynamicRoutes := fastroute.Chain(
		fastroute.New("/users/:id", handler),
		fastroute.New("/users/:id/roles", handler),
	)

	http.ListenAndServe(":8080", fastroute.Chain(staticRoutes, dynamicRoutes))
}
```

### Trailing slash or fixed path redirects

In cases when your API faces public, it might be a good idea to redirect with corrected
request URL if user makes a simple mistake.

This fixes trailing slash, case mismatch and cleaned path all at once. Note, we should
follow some specific rule, how we build our path patterns in order to be able to fix them.
In this case we follow **all lowercase** rule for static segments, parameters may match any
case.

``` go
package main

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/DATA-DOG/fastroute"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintln(w, req.URL.Path, fastroute.Parameters(req))
	})

	// we follow the lowercase rule for static segments
	router := fastroute.Chain(
		fastroute.New("/status", handler),
		fastroute.New("/users/:id", handler),
		fastroute.New("/users/:id/roles/", handler), // one with trailing slash
	)

	http.ListenAndServe(":8080", redirectTrailingOrFixedPath(router))

	// requesting: http://localhost:8080/Users/5/Roles
	// redirects: http://localhost:8080/users/5/roles/
}

func redirectTrailingOrFixedPath(router fastroute.Router) fastroute.Router {
	return fastroute.RouterFunc(func(req *http.Request) http.Handler {
		if h := router.Route(req); h != nil {
			return h // has matched, no need for fixing
		}

		p := strings.ToLower(path.Clean(req.URL.Path)) // first clean path and lowercase
		attempts := []string{p}                        // first variant with cleaned path
		if p[len(p)-1] == '/' {
			attempts = append(attempts, p[:len(p)-1]) // without trailing slash
		} else {
			attempts = append(attempts, p+"/") // with trailing slash
		}

		try, _ := http.NewRequest(req.Method, "/", nil) // make request for all attempts
		for _, attempt := range attempts {
			try.URL.Path = attempt
			if h := router.Route(try); h != nil {
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
				fastroute.Recycle(try)
				return redirect(strings.Join(fixed, "/"))
			}
		}
		return nil // could not fix path
	})
}

func redirect(fixedPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.URL.Path = fixedPath
		http.Redirect(w, req, req.URL.String(), http.StatusPermanentRedirect)
	})
}
```

### Named routes

This is trivial to implement a package inside your project, where all your routes used may be named.
And later paths built by these named routes from anywhere within your application.

``` go
package routes

import (
	"fmt"
	"strings"

	"github.com/DATA-DOG/fastroute"
)

var all = make(map[string]string)

func Named(name, path string, handler interface{}) fastroute.Router {
	if p, dup := all[name]; dup {
		panic(fmt.Sprintf(`route: "%s" at path: "%s" was already registered for path: "%s"`, name, path, p))
	}
	all[name] = path
	return fastroute.New(path, handler)
}

func Get(name string, params fastroute.Params) string {
	p, ok := all[name]
	if !ok {
		panic(fmt.Sprintf(`route: "%s" was never registered`, name))
	}
	for _, param := range params {
		if key := ":" + param.Key; strings.Index(p, key) != -1 {
			p = strings.Replace(p, key, param.Value, 1)
		} else if key = "*" + param.Key; strings.Index(p, key) != -1 {
			p = strings.Replace(p, key, param.Value, 1)
		}
	}

	if strings.IndexAny(p, ":*") != -1 {
		panic(fmt.Sprintf(`not all parameters were set: "%s" for route: "%s"`, p, name))
	}
	return p
}
```

Then the usage is obvious:

``` go
package main

import (
	"fmt"
	"net/http"

	"github.com/DATA-DOG/fastroute"
	"github.com/DATA-DOG/fastroute/routes" // should be somewhere in your project
)

func main() {
	router := fastroute.Chain(
		routes.Named("home", "/", handler),
		routes.Named("hello-full", "/hello/:name/:surname", handler),
	)

	fmt.Println(routes.Get("hello-full", fastroute.Params{
		{"name", "John"},
		{"surname", "Doe"},
	}))

	http.ListenAndServe(":8080", router)
}

func handler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(w, fmt.Sprintf(`%s "%s"`, req.Method, req.URL.Path))
}
```

### Hit counting frequently accessed routes

In cases where **n** number of routes is very high and it is unknown what routes
would be most frequently accessed or it changes during runtime, in order to
highly improve performance, you can use **hit count** based reordering middleware.

``` go
package main

import (
	"fmt"
	"net/http"
	"sort"
	"sync"

	fr "github.com/DATA-DOG/fastroute"
)

var routes = map[string]fr.Router{
	"GET": fr.Chain(
		// here follows frequently accessed routes
		HitCountingOrderedChain(
			fr.New("/", handler),
			fr.New("/health", handler),
			fr.New("/status", handler),
		),
		// less frequently accessed routes
		fr.New("/hello/:name/:surname", handler),
		fr.New("/hello/:name", handler),
	),
	"POST": fr.Chain(
		fr.New("/users", handler),
		fr.New("/users/:id", handler),
	),
}

// serves routes by request method
var router = fr.RouterFunc(func(req *http.Request) http.Handler {
	return routes[req.Method] // fastroute.Router is also http.Handler
})

func main() {
	http.ListenAndServe(":8080", router)
}

func HitCountingOrderedChain(routes ...fr.Router) fr.Router {
	type HitCounter struct {
		fr.Router
		hits int64
	}

	hitRoutes := make([]*HitCounter, len(routes))
	for i, r := range routes {
		hitRoutes[i] = &HitCounter{Router: r}
	}
	mu := sync.Mutex{}

	return fr.RouterFunc(func(req *http.Request) http.Handler {
		mu.Lock()
		defer mu.Unlock()
		for i, r := range hitRoutes {
			if h := r.Route(req); h != nil {
				r.hits++
				// reorder route hit is behind one third of routes
				if i > len(hitRoutes)*30/100 {
					sort.Slice(hitRoutes, func(i, j int) bool {
						return hitRoutes[i].hits > hitRoutes[j].hits
					})
				}
				return h
			}
		}
		return nil
	})
}

func handler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(w, fmt.Sprintf(
		`%s "%s", pattern: "%s", parameters: "%v"`,
		req.Method,
		req.URL.Path,
		fr.Pattern(req),
		fr.Parameters(req),
	))
}
```

## Benchmarks

The benchmarks can be [found here](https://github.com/l3pp4rd/go-http-routing-benchmark/tree/fastroute).

The output for: `go test -bench='Gin|HttpRouter|GorillaMux|FastRoute'`

Benchmark type            | repeats   | cpu time op    | mem op      | mem allocs op    |
--------------------------|----------:|---------------:|------------:|-----------------:|
Gin_Param                 | 20000000  |     70.3 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_Param          |   500000  |     3133 ns/op |    1056 B/op|      11 allocs/op|
HttpRouter_Param          | 20000000  |      119 ns/op |      32 B/op|       1 allocs/op|
**FastRoute_Param**       | 20000000  |     78.4 ns/op |       0 B/op|       0 allocs/op|
Gin_Param5                | 10000000  |      122 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_Param5         |   300000  |     4657 ns/op |    1184 B/op|      11 allocs/op|
HttpRouter_Param5         |  3000000  |      489 ns/op |     160 B/op|       1 allocs/op|
**FastRoute_Param5**      | 20000000  |      107 ns/op |       0 B/op|       0 allocs/op|
Gin_Param20               |  5000000  |      281 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_Param20        |   200000  |    11437 ns/op |    3547 B/op|      13 allocs/op|
HttpRouter_Param20        |  1000000  |     1690 ns/op |     640 B/op|       1 allocs/op|
**FastRoute_Param20**     | 10000000  |      204 ns/op |       0 B/op|       0 allocs/op|
Gin_ParamWrite            | 10000000  |      177 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_ParamWrite     |   500000  |     3197 ns/op |    1064 B/op|      12 allocs/op|
HttpRouter_ParamWrite     | 10000000  |      171 ns/op |      32 B/op|       1 allocs/op|
**FastRoute_ParamWrite**  | 10000000  |      125 ns/op |       0 B/op|       0 allocs/op|
Gin_GithubStatic          | 20000000  |     92.1 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_GithubStatic   |   100000  |    15488 ns/op |     736 B/op|      10 allocs/op|
HttpRouter_GithubStatic   | 30000000  |     50.9 ns/op |       0 B/op|       0 allocs/op|
**FastRoute_GithubStatic**| 30000000  |     42.0 ns/op |       0 B/op|       0 allocs/op|
Gin_GithubParam           | 10000000  |      168 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_GithubParam    |   200000  |    10178 ns/op |    1088 B/op|      11 allocs/op|
HttpRouter_GithubParam    |  5000000  |      304 ns/op |      96 B/op|       1 allocs/op|
**FastRoute_GithubParam** |  1000000  |     2202 ns/op |       0 B/op|       0 allocs/op|
Gin_GithubAll             |    50000  |    28518 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_GithubAll      |      300  |  5719143 ns/op |  211840 B/op|    2272 allocs/op|
HttpRouter_GithubAll      |    30000  |    51511 ns/op |   13792 B/op|     167 allocs/op|
**FastRoute_GithubAll**   |     5000  |   349434 ns/op |      11 B/op|       0 allocs/op|
Gin_GPlusStatic           | 20000000  |     75.4 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_GPlusStatic    |  1000000  |     1978 ns/op |     736 B/op|      10 allocs/op|
HttpRouter_GPlusStatic    | 50000000  |     30.3 ns/op |       0 B/op|       0 allocs/op|
**FastRoute_GPlusStatic** | 100000000 |     23.9 ns/op |       0 B/op|       0 allocs/op|
Gin_GPlusParam            | 20000000  |     94.8 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_GPlusParam     |   500000  |     4068 ns/op |    1056 B/op|      11 allocs/op|
HttpRouter_GPlusParam     | 10000000  |      215 ns/op |      64 B/op|       1 allocs/op|
**FastRoute_GPlusParam**  |  5000000  |      249 ns/op |       0 B/op|       0 allocs/op|
Gin_GPlus2Params          | 10000000  |      134 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_GPlus2Params   |   200000  |     8206 ns/op |    1088 B/op|      11 allocs/op|
HttpRouter_GPlus2Params   | 10000000  |      233 ns/op |      64 B/op|       1 allocs/op|
**FastRoute_GPlus2Params**|  3000000  |      438 ns/op |       0 B/op|       0 allocs/op|
Gin_GPlusAll              |  1000000  |     1296 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_GPlusAll       |    20000  |    67092 ns/op |   13296 B/op|     142 allocs/op|
HttpRouter_GPlusAll       |   500000  |     2332 ns/op |     640 B/op|      11 allocs/op|
**FastRoute_GPlusAll**    |   500000  |     3417 ns/op |       0 B/op|       0 allocs/op|
Gin_ParseStatic           | 20000000  |     72.7 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_ParseStatic    |   500000  |     2951 ns/op |     752 B/op|      11 allocs/op|
HttpRouter_ParseStatic    | 50000000  |     31.9 ns/op |       0 B/op|       0 allocs/op|
**FastRoute_ParseStatic** | 50000000  |     30.0 ns/op |       0 B/op|       0 allocs/op|
Gin_ParseParam            | 20000000  |     80.2 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_ParseParam     |   500000  |     3644 ns/op |    1088 B/op|      12 allocs/op|
HttpRouter_ParseParam     | 10000000  |      180 ns/op |      64 B/op|       1 allocs/op|
**FastRoute_ParseParam**  |  5000000  |      256 ns/op |       0 B/op|       0 allocs/op|
Gin_Parse2Params          | 20000000  |     93.9 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_Parse2Params   |   500000  |     3945 ns/op |    1088 B/op|      11 allocs/op|
HttpRouter_Parse2Params   | 10000000  |      205 ns/op |      64 B/op|       1 allocs/op|
**FastRoute_Parse2Params**| 10000000  |      212 ns/op |       0 B/op|       0 allocs/op|
Gin_ParseAll              |  1000000  |     2389 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_ParseAll       |    10000  |   125536 ns/op |   24864 B/op|     292 allocs/op|
HttpRouter_ParseAll       |   500000  |     3151 ns/op |     640 B/op|      16 allocs/op|
**FastRoute_ParseAll**    |   500000  |     3874 ns/op |       0 B/op|       0 allocs/op|
Gin_StaticAll             |   100000  |    19688 ns/op |       0 B/op|       0 allocs/op|
GorillaMux_StaticAll      |     1000  |  1561137 ns/op |  115648 B/op|    1578 allocs/op|
**FastRoute_StaticAll**   |   200000  |     7009 ns/op |       0 B/op|       0 allocs/op|
HttpRouter_StaticAll      |   200000  |    11083 ns/op |       0 B/op|       0 allocs/op|

We can see that **FastRoute** outperforms fastest routers in some of the cases:
- Number of routes is small.
- Routes are static and served from a map.
- There are many named parameters in route.

**FastRoute** was easily adapted for this benchmark. Where static routes are served, nothing
is better or faster than a static path **map**. **FastRoute** allows to build any kind of router,
depending on an use case. By default it targets smaller number of routes and the weakest
link is large set of dynamic routes, because these are matched one by one in order.

It always boils down to targeted case implementation. It is a general purpose router of
**172** lines of source code in one file, which can be copied, understood and adapted in
separate projects.

## Contributions

Feel free to open a pull request. Note, if you wish to contribute an extension to public (exported methods or types) -
please open an issue before to discuss whether these changes can be accepted. All backward incompatible changes are
and will be treated cautiously.

## License

**FastRoute** is licensed under the [three clause BSD license][license]

[license]: http://en.wikipedia.org/wiki/BSD_licenses "The three clause BSD license"
