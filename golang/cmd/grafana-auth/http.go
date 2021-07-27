package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	ginopentracing "github.com/Bose/go-gin-opentracing"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/grafana-auth/grafana/api/user"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var FactoryInputAPIKey string
var FactoryInputUser string

func SetupRestAPI(jaegerHost string, jaegerPort string) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Add a ginzap middleware, which:
	//   - Logs all requests, like a combined access and error log.
	//   - Logs to stdout.
	//   - RFC3339 with UTC time format.
	router.Use(ginzap.Ginzap(zap.L(), time.RFC3339, true))

	// Logs all panic to error log
	//   - stack means whether output the stack info.
	router.Use(ginzap.RecoveryWithZap(zap.L(), true))

	// initialize the global singleton for tracing...
	tracer, reporter, closer, err := ginopentracing.InitTracing("factoryinsight", jaegerHost+":"+jaegerPort, ginopentracing.WithEnableInfoLog(false))
	if err != nil {
		panic("unable to init tracing")
	}
	defer func(closer io.Closer) {
		err := closer.Close()
		if err != nil {
			panic(err)
		}
	}(closer)
	defer reporter.Close()
	opentracing.SetGlobalTracer(tracer)

	// create the middleware
	p := ginopentracing.OpenTracer([]byte("api-request-"))

	// tell gin to use the middleware
	router.Use(p)

	// Healthcheck
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "online")
	})

	// Version of the API
	v1 := router.Group("/api/v1")
	{
		v1.GET("/:service/*data", getProxyHandler)
	}

	err = router.Run(":80")
	if err != nil {
		panic(err)
	}
}

func handleInvalidInputError(parentSpan opentracing.Span, c *gin.Context, err error) {

	ext.LogError(parentSpan, err)
	traceID, _ := internal.ExtractTraceID(parentSpan)

	zap.S().Errorw("Invalid input error",
		"error", err,
		"trace id", traceID,
	)

	c.String(400, "You have provided a wrong input. Please check your parameters and mention the following trace id while contacting our support: "+traceID)
}

type getProxyRequest struct {
	Service     string `uri:"service" binding:"required"`
	OriginalURI string `uri:"data" binding:"required"`
}

func getProxyHandler(c *gin.Context) {
	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "getProxyHandler", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "getProxyHandler", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getProxyRequest getProxyRequest
	var err error

	// Failed to parse request into service name and original url
	err = c.BindUri(&getProxyRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Grafana session not present in request
	session, err := c.Cookie("grafana_session")
	if err != nil {

		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// Check if user is logged in
	loggedIn, err := CheckUserLoggedIn(session)
	if err != nil {
		handleInvalidInputError(span, c, err)
	}

	// Abort if not logged in
	if !loggedIn {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// Switch to handle our services
	switch getProxyRequest.Service {
	case "factoryinput":
		HandleFactoryInput(c, session, getProxyRequest)
	default:
		c.AbortWithStatus(http.StatusBadRequest)
	}
}

// HandleFactoryInput handles proxy requests to factoryinput
func HandleFactoryInput(c *gin.Context, sessioncookie string, request getProxyRequest) {
	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "HandleFactoryInput", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "HandleFactoryInput", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	proxyUrl := strings.TrimPrefix(string(request.OriginalURI), "/")

	// Validate proxy url
	u, err := url.Parse(proxyUrl)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Split proxy url into customer, location, asset, value
	s := strings.Split(u.Path, "/")
	if len(s) != 5 {
		handleInvalidInputError(span, c, errors.New(fmt.Sprintf("factoryinput url invalid: %d", len(s))))
		return
	}

	customer := s[1]
	/*
		location := s[2]
		asset := s[3]
		value := s[4]
	*/

	// Get grafana organizations of user
	orgas, err := user.GetOrgas(sessioncookie)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Check if user is in orga, with same name as customer
	allowedOrg := false
	for _, orgsElement := range orgas {
		if orgsElement.Name == customer {
			allowedOrg = true
			break
		}
	}

	// Abort if not in allowed org
	if !allowedOrg {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// Proxy request to backend
	client := &http.Client{}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {

		return
	}

	// Add headers for backend
	req.Header.Set("Cookie", fmt.Sprintf("grafana_session=%s", sessioncookie))
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", FactoryInputUser, FactoryInputAPIKey)))))

	resp, err := client.Do(req)
	if err != nil {

		return
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			panic(err)
		}
	}(resp.Body)

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {

		log.Fatal(err)
	}

	c.Status(resp.StatusCode)

	for a, b := range resp.Header {
		c.Header(a, b[0])
	}
	// Add cors headers for reply to original requester
	c.Header("Access-Control-Allow-Headers", "*")

	_, err = c.Writer.Write(bodyBytes)
	if err != nil {
		err = c.AbortWithError(http.StatusInternalServerError, err)
		if err != nil {
			panic(err)
		}
	}
}
