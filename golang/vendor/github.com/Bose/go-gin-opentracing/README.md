# go-gin-opentracing
[![](https://godoc.org/github.com/Bose/go-gin-opentracing?status.svg)](https://godoc.org/github.com/Bose/go-gin-opentracing) 
[![Go Report Card](https://goreportcard.com/badge/github.com/Bose/go-gin-opentracing)](https://goreportcard.com/report/github.com/Bose/go-gin-opentracing)
[![Release](https://img.shields.io/github/release/Bose/go-gin-opentracing.svg?style=flat-square)](https://github.com/Bose/go-gin-opentracing/releases) 

OpenTracing middleware from the [Gin http framework](https://github.com/gin-gonic/gin).

Based on [opentracing-go](https://github.com/opentracing/opentracing-go), this middleware adds an OpenTracing span for every http request, using the inbound requestID (if one exists) and adds the request span to the gin.Context as the key: `tracing-context`.


## Required Reading
In order to understand the Go platform API, one must first be familiar with the [OpenTracing project](https://opentracing.io/) and [terminology](https://opentracing.io/specification/) more specifically.



## Installation

`$ go get github.com/Bose/go-gin-opentracing`

## Usage as middleware

```go
package main

import (
	"fmt"
	"os"

	ginopentracing "github.com/Bose/go-gin-opentracing"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
)

func main() {
	r := gin.Default()
	hostName, err := os.Hostname()
	if err != nil {
		hostName = "unknown"
	}
	// initialize the global singleton for tracing...
	tracer, reporter, closer, err := ginopentracing.InitTracing(fmt.Sprintf("go-gin-opentracing-example::%s", hostName), "localhost:5775", ginopentracing.WithEnableInfoLog(true))
	if err != nil {
		panic("unable to init tracing")
	}
	defer closer.Close()
	defer reporter.Close()
	opentracing.SetGlobalTracer(tracer)

	// create the middleware
	p := ginopentracing.OpenTracer([]byte("api-request-"))

	// tell gin to use the middleware
	r.Use(p)

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, "Hello world!")
	})

	r.Run(":29090")
}

```

See the [example.go file](https://github.com/Bose/go-gin-opentracing/blob/master/example/example.go)

## Usage within gin.Handler

Within the gin Handler you can access this request span and use it to create additional spans, and/or add span tags. 
```go
func HelloWorld(c *gin.Context) {
	if cspan, ok := c.Get("tracing-context"); ok {
		span = tracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "helloword", c.Request.Method, c.Request.URL.Path)	
	} else {
		span = tracing.StartSpanWithHeader(&c.Request.Header, "helloworld", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()
	c.String(200, "Hello.")
}
```

