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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/grafana-proxy/grafana/api/user"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var FactoryInputAPIKey string
var FactoryInputUser string
var FactoryInputBaseURL string
var FactoryInsightBaseUrl string

const tracingContext = "tracing-context"

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

	const serviceRoute = "/:service/*data"
	// Version of the API
	v1 := router.Group("/api/v1")
	{
		v1.GET(serviceRoute, getProxyHandler)
		v1.POST(serviceRoute, postProxyHandler)
		v1.OPTIONS(serviceRoute, optionsCORSHAndler)
	}

	err = router.Run(":80")
	if err != nil {
		panic(err)
	}
}

func optionsCORSHAndler(c *gin.Context) {
	zap.S().Debugf("optionsCORSHAndler")
	AddCorsHeaders(c)
	c.Status(http.StatusOK)
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

type getProxyRequestPath struct {
	Service     string `uri:"service" binding:"required"`
	OriginalURI string `uri:"data" binding:"required"`
}

func handleProxyRequest(c *gin.Context, method string) {
	zap.S().Debugf("getProxyHandler")
	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get(tracingContext); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "getProxyHandler", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "getProxyHandler", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getProxyRequestPath getProxyRequestPath
	var err error

	// Failed to parse request into service name and original url
	err = c.BindUri(&getProxyRequestPath)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	match, err := regexp.Match("[a-zA-z0-9_\\-?=/]+", []byte(getProxyRequestPath.OriginalURI))
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}
	// Invalid url
	if !match {
		handleInvalidInputError(span, c, err)
		return
	}

	zap.S().Debugf("", c.Request)

	// Switch to handle our services
	switch getProxyRequestPath.Service {
	case "factoryinput":
		HandleFactoryInput(c, getProxyRequestPath, method)
	case "factoryinsight":
		HandleFactoryInsight(c, getProxyRequestPath, method)
	default:
		{
			zap.S().Warnf("getProxyRequestPath.Service", getProxyRequestPath.Service)
			c.AbortWithStatus(http.StatusBadRequest)
		}
	}
}

func AddCorsHeaders(c *gin.Context) {
	origin := c.GetHeader("Origin")
	if len(origin) == 0 {
		zap.S().Debugf("Add cors wildcard")
		origin = "*"
	} else {
		zap.S().Debugf("Set cors origin to: %s", origin)
	}
	c.Header("Access-Control-Allow-Headers", "content-type, Authorization")
	c.Header("Access-Control-Allow-Credentials", "true")
	c.Header("Access-Control-Allow-Origin", origin)
	c.Header("Access-Control-Allow-Methods", "*")
}

func postProxyHandler(c *gin.Context) {
	// Add cors headers for reply to original requester
	AddCorsHeaders(c)
	handleProxyRequest(c, "POST")
}

func getProxyHandler(c *gin.Context) {
	// Add cors headers for reply to original requester
	AddCorsHeaders(c)
	handleProxyRequest(c, "GET")
}

func IsBase64(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

func HandleFactoryInsight(c *gin.Context, request getProxyRequestPath, method string) {
	zap.S().Debugf("HandleFactoryInsight")
	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get(tracingContext); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "HandleFactoryInsight", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "HandleFactoryInsight", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	authHeader := c.GetHeader("authorization")
	s := strings.Split(authHeader, " ")
	//Basic BASE64Encoded
	if len(s) != 2 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if IsBase64(s[1]) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// HTTP Basic auth not present in request
	if authHeader == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	proxyUrl := strings.TrimPrefix(string(request.OriginalURI), "/")

	var path = "?"
	for k, v := range c.Request.URL.Query() {
		path += fmt.Sprintf("%s=%s&", k, v[0])
	}

	if len(path) == 1 {
		path = ""
	}

	zap.S().Debugf("FactoryInsightBaseUrl: ", FactoryInsightBaseUrl)
	zap.S().Debugf("proxyUrl: ", proxyUrl)
	zap.S().Debugf("path: ", path)

	// Validate proxy url
	u, err := url.Parse(fmt.Sprintf("%s%s%s", FactoryInsightBaseUrl, proxyUrl, path))

	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	DoProxiedRequest(c, err, u, "", authHeader, method)
}

// HandleFactoryInput handles proxy requests to factoryinput
func HandleFactoryInput(c *gin.Context, request getProxyRequestPath, method string) {
	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get(tracingContext); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "HandleFactoryInput", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "HandleFactoryInput", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	zap.S().Warnf("HandleFactoryInput")

	// Grafana sessionCookie not present in request
	sessionCookie, err := c.Cookie("grafana_session")
	if err != nil {
		zap.S().Warnf("No grafana_session in cookie !")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	zap.S().Warnf("CheckUserLoggedIn")
	// Check if user is logged in
	loggedIn, err := CheckUserLoggedIn(sessionCookie)
	if err != nil {
		zap.S().Warnf("Login error")
		handleInvalidInputError(span, c, err)
	}

	// Abort if not logged in
	if !loggedIn {
		zap.S().Warnf("StatusForbidden")
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	proxyUrl := strings.TrimPrefix(string(request.OriginalURI), "/")

	zap.S().Warnf("ValidateProxyUrl")
	// Validate proxy url
	u, err := url.Parse(fmt.Sprintf("%s%s", FactoryInputBaseURL, proxyUrl))
	if err != nil {
		zap.S().Warnf("url.Parse failed", fmt.Sprintf("%s%s", FactoryInputBaseURL, proxyUrl))
		handleInvalidInputError(span, c, err)
		return
	}

	// Split proxy url into customer, location, asset, value
	s := strings.Split(u.Path, "/")
	if len(s) != 5 {
		zap.S().Warnf("String split failed", len(s))
		handleInvalidInputError(span, c, errors.New(fmt.Sprintf("factoryinput url invalid: %d", len(s))))
		return
	}

	customer := s[1]
	/*
		location := s[2]
		asset := s[3]
		value := s[4]
	*/

	zap.S().Warnf("GetOrgas")
	// Get grafana organizations of user
	orgas, err := user.GetOrgas(sessionCookie)
	if err != nil {
		zap.S().Warnf("GetOrgas failed", err, sessionCookie)
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
		zap.S().Warnf("!allowedOrg")
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	ak := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", FactoryInputUser, FactoryInputAPIKey))))
	DoProxiedRequest(c, err, u, sessionCookie, ak, method)
}

func DoProxiedRequest(c *gin.Context, err error, u *url.URL, sessionCookie string, authorizationKey string, method string) {
	// Proxy request to backend
	client := &http.Client{}

	zap.S().Debugf("Request URL: ", u.String())
	//CORS request !
	if u.String() == "" {
		zap.S().Debugf("CORS Answer")
		c.Status(http.StatusOK)
		_, err := c.Writer.Write([]byte("online"))
		if err != nil {
			zap.S().Debugf("Failed to reply to CORS request")
			c.AbortWithError(http.StatusInternalServerError, err)
		}
	} else {
		req, err := http.NewRequest(method, u.String(), nil)
		if err != nil {
			zap.S().Debugf("Request error: ", err)
			return
		}
		// Add headers for backend
		if len(sessionCookie) > 0 {
			req.Header.Set("Cookie", fmt.Sprintf("grafana_session=%s", sessionCookie))
		}
		req.Header.Set("Authorization", authorizationKey)

		resp, err := client.Do(req)
		if err != nil {
			zap.S().Debugf("Client.Do error: ", err)
			c.AbortWithStatus(500)
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

		zap.S().Debugf("Backend answer:")
		zap.S().Debugf(string(bodyBytes))

		c.Status(resp.StatusCode)

		for a, b := range resp.Header {
			c.Header(a, b[0])
		}
		_, err = c.Writer.Write(bodyBytes)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
		}
	}

	if err != nil {
		err = c.AbortWithError(http.StatusInternalServerError, err)
		if err != nil {
			panic(err)
		}
	}
}
