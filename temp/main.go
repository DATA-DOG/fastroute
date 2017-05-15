package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/DATA-DOG/fastroute"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintln(w, req.URL.Path, fastroute.Parameters(req))
	})

	// lets say our API strategy is to have all paths
	// lowercased and a trailing slash
	routes := fastroute.New(
		fastroute.Route("/status/", handler),
		fastroute.Route("/users/:id/", handler),
		fastroute.Route("/users/:id/roles/", handler),
	)

	router := fastroute.RouterFunc(func(req *http.Request) http.Handler {
		if h := routes.Match(req); h != nil {
			return h // has matched, no need for fixing
		}

		p := req.URL.Path
		if p[len(p)-1] != '/' {
			p += "/" // had no trailing slash
		}

		// clone request for testing
		r := new(http.Request)
		*r = *req
		r.URL.Path = strings.ToLower(p) // maybe captain CAPS LOCK?

		if matched, _ := fastroute.Handles(routes, r); matched {
			return redirect(r.URL.Path) // fixed trailing slash
		}

		return nil
	})

	http.ListenAndServe(":8080", router)
}

func redirect(fixedPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.URL.Path = fixedPath
		http.Redirect(w, req, req.URL.String(), http.StatusPermanentRedirect)
	})
}
