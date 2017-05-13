[![Build Status](https://travis-ci.org/DATA-DOG/fastroute.svg?branch=master)](https://travis-ci.org/DATA-DOG/fastroute)
[![GoDoc](https://godoc.org/github.com/DATA-DOG/fastroute?status.svg)](https://godoc.org/github.com/DATA-DOG/fastroute)
[![codecov.io](https://codecov.io/github/DATA-DOG/fastroute/branch/master/graph/badge.svg)](https://codecov.io/github/DATA-DOG/fastroute)

# FastRoute

Insanely **fast** and **robust** http router for golang. Only **200**
lines of code.

``` go
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/DATA-DOG/fastroute"
)

func Index(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Welcome!\n")
}

func Hello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello, %s!\n", fastroute.Parameters(r).ByName("name"))
}

func main() {
	log.Fatal(http.ListenAndServe(":8080", fastroute.New(
		fastroute.Route("/", Index),
		fastroute.Route("/hello/:name", Hello),
	)))
}
```

This deserves a quote from **Rob Pike**:

> Fancy algorithms are slow when n is small, and n is usually small. Fancy
> algorithms have big constants. Until you know that n is frequently going
> to be big, don't get fancy.

The trade off this router makes is the size of **big n**. But it provides
orthogonal building blocks, just like **http.Handler** does, to build
customized routers.

By default this router does not provide:

1. Route based on HTTP method. Because it is simple to manage that in
   handler or middleware.
2. Trailing slash redirects. Because that is simple to add a middleware
   for your preferred choice, before any route is looked up.
3. Fixed path redirects, you rarely need this for internal APIs or micro
   services. And if someone makes such a mistake, it would be costly to
   produce that redirect.
4. Method not found handlers, these may be custom and in some cases not
   useful at all.

In overall, this router is suitable for internal service APIs or serve as
building blocks for more complex and customized routers.

## Benchmarks

The benchmarks can be [found here](https://github.com/l3pp4rd/go-http-routing-benchmark/tree/fastroute).


Benchmark type             | repeats | cpu time per op | mem usage op  | mem allocs op    |
---------------------------|--------:|----------------:|--------------:|-----------------:|
Gin_Param                  |20000000 |      72.8 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_Param           |  500000 |      3118 ns/op |   1056 B/op   |     11 allocs/op |
HttpRouter_Param           |20000000 |       119 ns/op |     32 B/op   |      1 allocs/op |
**FastRoute_Param**        |20000000 |      74.8 ns/op |      0 B/op   |      0 allocs/op |
Pat_Param                  | 1000000 |      1911 ns/op |    648 B/op   |     12 allocs/op |
Gin_Param5                 |10000000 |       122 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_Param5          |  300000 |      4598 ns/op |   1184 B/op   |     11 allocs/op |
HttpRouter_Param5          | 3000000 |       485 ns/op |    160 B/op   |      1 allocs/op |
**FastRoute_Param5**       |20000000 |      98.4 ns/op |      0 B/op   |      0 allocs/op |
Pat_Param5                 |  300000 |      4668 ns/op |    964 B/op   |     32 allocs/op |
Gin_Param20                | 5000000 |       286 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_Param20         |  200000 |     11181 ns/op |   3548 B/op   |     13 allocs/op |
HttpRouter_Param20         | 1000000 |      1672 ns/op |    640 B/op   |      1 allocs/op |
**FastRoute_Param20**      |10000000 |       194 ns/op |      0 B/op   |      0 allocs/op |
Pat_Param20                |   50000 |     20649 ns/op |   4687 B/op   |    111 allocs/op |
Gin_ParamWrite             |10000000 |       178 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_ParamWrite      |  500000 |      3139 ns/op |   1064 B/op   |     12 allocs/op |
HttpRouter_ParamWrite      |10000000 |       158 ns/op |     32 B/op   |      1 allocs/op |
**FastRoute_ParamWrite**   |10000000 |       130 ns/op |      0 B/op   |      0 allocs/op |
Pat_ParamWrite             |  500000 |      3211 ns/op |   1072 B/op   |     17 allocs/op |
Gin_GithubStatic           |20000000 |      88.6 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_GithubStatic    |  100000 |     15040 ns/op |    736 B/op   |     10 allocs/op |
HttpRouter_GithubStatic    |30000000 |      49.7 ns/op |      0 B/op   |      0 allocs/op |
**FastRoute_GithubStatic** |  500000 |      3103 ns/op |      0 B/op   |      0 allocs/op |
Pat_GithubStatic           |  200000 |     10912 ns/op |   3648 B/op   |     76 allocs/op |
Gin_GithubParam            |10000000 |       141 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_GithubParam     |  200000 |     10051 ns/op |   1088 B/op   |     11 allocs/op |
HttpRouter_GithubParam     | 5000000 |       301 ns/op |     96 B/op   |      1 allocs/op |
**FastRoute_GithubParam**  | 2000000 |       722 ns/op |      0 B/op   |      0 allocs/op |
Pat_GithubParam            |  200000 |      7030 ns/op |   2464 B/op   |     48 allocs/op |
Gin_GithubAll              |   50000 |     27829 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_GithubAll       |     300 |   5658862 ns/op | 211840 B/op   |   2272 allocs/op |
HttpRouter_GithubAll       |   30000 |     50044 ns/op |  13792 B/op   |    167 allocs/op |
**FastRoute_GithubAll**    |    3000 |    443539 ns/op |   2253 B/op   |      0 allocs/op |
Pat_GithubAll              |     300 |   4419366 ns/op |1499571 B/op   |  27435 allocs/op |
Gin_GPlusStatic            |20000000 |      72.4 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_GPlusStatic     | 1000000 |      1985 ns/op |    736 B/op   |     10 allocs/op |
HttpRouter_GPlusStatic     |50000000 |      30.4 ns/op |      0 B/op   |      0 allocs/op |
**FastRoute_GPlusStatic**  |20000000 |      77.5 ns/op |      0 B/op   |      0 allocs/op |
Pat_GPlusStatic            | 5000000 |       330 ns/op |     96 B/op   |      2 allocs/op |
Gin_GPlusParam             |20000000 |      92.5 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_GPlusParam      |  300000 |      4039 ns/op |   1056 B/op   |     11 allocs/op |
HttpRouter_GPlusParam      |10000000 |       209 ns/op |     64 B/op   |      1 allocs/op |
**FastRoute_GPlusParam**   |20000000 |      93.4 ns/op |      0 B/op   |      0 allocs/op |
Pat_GPlusParam             | 1000000 |      1940 ns/op |    688 B/op   |     12 allocs/op |
Gin_GPlus2Params           |10000000 |       119 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_GPlus2Params    |  200000 |      8202 ns/op |   1088 B/op   |     11 allocs/op |
HttpRouter_GPlus2Params    |10000000 |       231 ns/op |     64 B/op   |      1 allocs/op |
**FastRoute_GPlus2Params** | 5000000 |       349 ns/op |      0 B/op   |      0 allocs/op |
Pat_GPlus2Params           |  300000 |      6533 ns/op |   2256 B/op   |     34 allocs/op |
Gin_GPlusAll               | 1000000 |      1259 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_GPlusAll        |   20000 |     66650 ns/op |  13296 B/op   |    142 allocs/op |
HttpRouter_GPlusAll        |  500000 |      2326 ns/op |    640 B/op   |     11 allocs/op |
**FastRoute_GPlusAll**     |  500000 |      4029 ns/op |    177 B/op   |      0 allocs/op |
Pat_GPlusAll               |   30000 |     47447 ns/op |  16576 B/op   |    298 allocs/op |
Gin_ParseStatic            |20000000 |      73.7 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_ParseStatic     |  500000 |      2907 ns/op |    752 B/op   |     11 allocs/op |
HttpRouter_ParseStatic     |50000000 |      31.8 ns/op |      0 B/op   |      0 allocs/op |
**FastRoute_ParseStatic**  |10000000 |       193 ns/op |      0 B/op   |      0 allocs/op |
Pat_ParseStatic            | 2000000 |       782 ns/op |    240 B/op   |      5 allocs/op |
Gin_ParseParam             |20000000 |      78.1 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_ParseParam      |  500000 |      3605 ns/op |   1088 B/op   |     12 allocs/op |
HttpRouter_ParseParam      |10000000 |       178 ns/op |     64 B/op   |      1 allocs/op |
**FastRoute_ParseParam**   |10000000 |       145 ns/op |      0 B/op   |      0 allocs/op |
Pat_ParseParam             |  500000 |      3175 ns/op |   1120 B/op   |     17 allocs/op |
Gin_Parse2Params           |20000000 |      92.1 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_Parse2Params    |  500000 |      3918 ns/op |   1088 B/op   |     11 allocs/op |
HttpRouter_Parse2Params    |10000000 |       212 ns/op |     64 B/op   |      1 allocs/op |
**FastRoute_Parse2Params** |20000000 |       101 ns/op |      0 B/op   |      0 allocs/op |
Pat_Parse2Params           |  500000 |      2981 ns/op |    832 B/op   |     17 allocs/op |
Gin_ParseAll               | 1000000 |      2284 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_ParseAll        |   10000 |    124577 ns/op |  24864 B/op   |    292 allocs/op |
HttpRouter_ParseAll        |  500000 |      3117 ns/op |    640 B/op   |     16 allocs/op |
**FastRoute_ParseAll**     |  200000 |      6618 ns/op |    698 B/op   |      0 allocs/op |
Pat_ParseAll               |   30000 |     56575 ns/op |  17264 B/op   |    343 allocs/op |
Gin_StaticAll              |  100000 |     19595 ns/op |      0 B/op   |      0 allocs/op |
GorillaMux_StaticAll       |    1000 |   1532075 ns/op | 115648 B/op   |   1578 allocs/op |
**FastRoute_StaticAll**    |   10000 |    102046 ns/op |      0 B/op   |      0 allocs/op |
Pat_StaticAll              |    1000 |   1591701 ns/op | 533904 B/op   |  11123 allocs/op |
HttpRouter_StaticAll       |  200000 |     10654 ns/op |      0 B/op   |      0 allocs/op |

We can see that it degrades with a number of routes, because routes are
served by matching a slice in order. **Gin** and all **HttpRouter** based
implementations are using trie (radix tree). If you have thousands of
routes, then it makes sense to customize the default implementation and
at least use a request method or first path segment based map.

## Contributions

Feel free to open a pull request. Note, if you wish to contribute an extension to public (exported methods or types) -
please open an issue before to discuss whether these changes can be accepted. All backward incompatible changes are
and will be treated cautiously.

## License

**FastRoute** is licensed under the [three clause BSD license][license]

[license]: http://en.wikipedia.org/wiki/BSD_licenses "The three clause BSD license"
