package router

import (
	"context"
	"net/http"
	"strings"
)

type Router interface {
	http.Handler
	Route(*http.Request) http.Handler
}

type RouterFunc func(*http.Request) http.Handler

func (rf RouterFunc) Route(r *http.Request) http.Handler {
	return rf(r)
}

func (rf RouterFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := rf(r)
	if nil == h {
		h = http.NotFoundHandler()
	}
	h.ServeHTTP(w, r)
}

func Get(p string, h http.Handler) Router {
	return Method("GET", Path(p, h))
}

func Post(p string, h http.Handler) Router {
	return Method("POST", Path(p, h))
}

func New(routes ...Router) Router {
	return RouterFunc(func(r *http.Request) http.Handler {
		var found http.Handler
		for _, router := range routes {
			if found = router.Route(r); found != nil {
				break
			}
		}
		return found
	})
}

func Method(method string, router Router) Router {
	m := strings.ToUpper(method)
	allow := strings.Join([]string{m, "OPTIONS", "HEAD"}, ",")

	return RouterFunc(func(r *http.Request) http.Handler {
		if r.Method != m && r.Method != "OPTIONS" && r.Method != "HEAD" {
			return nil
		}

		h := router.Route(r)
		if h == nil {
			return nil
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "OPTIONS" {
				w.Header().Set("Allow", allow)
				return
			}

			h.ServeHTTP(w, r)
		})
	})
}

func Path(path string, handler http.Handler) Router {
	p := "/" + strings.TrimLeft(path, "/")

	// maybe static route
	if strings.IndexAny(p, ":*") == -1 {
		return RouterFunc(func(r *http.Request) http.Handler {
			if p == r.URL.Path {
				return handler
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

	// dynamic route matcher
	return RouterFunc(func(r *http.Request) http.Handler {
		if ctx := match(p, r.URL.Path, r.Context()); ctx != nil {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				handler.ServeHTTP(w, req.WithContext(ctx))
			})
		}
		return nil
	})
}

func next(path string) int {
	if i := strings.IndexByte(path[1:], '/'); i != -1 {
		return i + 1
	}
	return len(path) // last path segment
}

func match(pat, url string, ctx context.Context) context.Context {
	if len(pat) <= 1 || len(url) <= 1 {
		if pat == url {
			return ctx
		}
		return nil
	}

	i, j := next(pat), next(url)

	switch {
	case pat[1] == ':':
		ctx = context.WithValue(ctx, pat[2:i], url[1:j])
	case pat[1] == '*':
		return context.WithValue(ctx, pat[2:i], url[1:len(url)])
	case pat[:i] != url[:j]:
		return nil
	}

	return match(pat[i:], url[j:], ctx)
}
