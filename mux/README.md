# Full featured HTTP router

Is based on light routing package **fastroute**. Shares similar
performance with fastest routers, when a number of dynamic routes is not
large. Is order of magnitude faster than gorilla mux, but shares same
features. Though it fits in **200** loc.

The core differences to Trie (radix tree) based routers:

- Allows routes like **/user/new** and **/user/:user** together.
- Fits in one source file and is much more extensible.
- Uses standard **http.Handler** so you can nest any 3rd party middleware
  without having to re-implement it.
- May fix both, trailing slash and path for redirect.

This package is just an example of **fastroute** implementation. Which is
intended to be copied or stripped down to some specific use cases.

``` go
package main

import (
    "fmt"
    "log"
    "net/http"
    "github.com/DATA-DOG/fastroute/mux"
)

func Index(w http.ResponseWriter, r *http.Request) {
    fmt.Fprint(w, "Welcome!\n")
}

func Hello(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "hello, %s!\n", fastroute.Parameters(r).ByName("name"))
}

func main() {
    router := mux.New()
    router.GET("/", Index)
    router.GET("/hello/:name", Hello)

    log.Fatal(http.ListenAndServe(":8080", router.Server()))
}
```
