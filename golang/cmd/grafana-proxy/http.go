package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/grafana-proxy/grafana/api/user"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	stdout "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	_ "go.opentelemetry.io/otel/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
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

var tracer = otel.Tracer("grafana-proxy-server")

func initTracer() *sdktrace.TracerProvider {
	exporter, err := stdout.New(stdout.WithPrettyPrint())
	if err != nil {
		panic(err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp
}

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

	// Setting up the tracer
	tp := initTracer()

	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			panic(fmt.Sprintf("Error shutting down tracer provider: %v", err))
		}
	}()
	// tell gin to use the middleware
	router.Use(otelgin.Middleware("grafana-proxy"))

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

	err := router.Run(":80")
	if err != nil {
		panic(err)
	}
}

func optionsCORSHAndler(c *gin.Context) {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "optionsCORSHAndler", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	zap.S().Debugf("optionsCORSHAndler")
	AddCorsHeaders(c)
	c.Status(http.StatusOK)
}

func handleInvalidInputError(c *gin.Context, err error) {

	traceID := "Failed to get traceID"
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "handleInvalidInputError", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
		traceID = span.SpanContext().SpanID().String()
	}

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
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "handleProxyRequest", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	zap.S().Debugf("getProxyHandler")

	var getProxyRequestPath getProxyRequestPath
	var err error

	// Failed to parse request into service name and original url
	err = c.BindUri(&getProxyRequestPath)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}
	var bodyBytes []byte
	if c.Request.Body != nil {
		bodyBytes, err = ioutil.ReadAll(c.Request.Body)
		if err != nil {
			handleInvalidInputError(c, err)
			return
		}
	}
	zap.S().Warnf("Read %d bytes from original request", len(bodyBytes))

	match, err := regexp.Match("[a-zA-z0-9_\\-?=/]+", []byte(getProxyRequestPath.OriginalURI))
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}
	// Invalid url
	if !match {
		handleInvalidInputError(c, err)
		return
	}

	zap.S().Debugf("", c.Request)

	// Switch to handle our services
	switch getProxyRequestPath.Service {
	case "factoryinput":
		HandleFactoryInput(c, getProxyRequestPath, method, bodyBytes)
	case "factoryinsight":
		HandleFactoryInsight(c, getProxyRequestPath, method, bodyBytes)
	default:
		{
			zap.S().Warnf("getProxyRequestPath.Service: %s", internal.SanitizeString(getProxyRequestPath.Service))
			c.AbortWithStatus(http.StatusBadRequest)
		}
	}
}

func AddCorsHeaders(c *gin.Context) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "AddCorsHeaders", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	origin := c.GetHeader("Origin")
	zap.S().Debugf("Requesting origin: %s", internal.SanitizeString(origin))
	if len(origin) == 0 {
		zap.S().Debugf("Add cors wildcard")
		origin = "*"
	} else {
		zap.S().Debugf("Set cors origin to: %s", internal.SanitizeString(origin))
	}
	c.Header("Access-Control-Allow-Headers", "content-type, Authorization")
	c.Header("Access-Control-Allow-Credentials", "true")
	c.Header("Access-Control-Allow-Origin", internal.SanitizeString(origin))
	c.Header("Access-Control-Allow-Methods", "*")
}

func postProxyHandler(c *gin.Context) {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "postProxyHandler", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	// Add cors headers for reply to original requester
	AddCorsHeaders(c)
	handleProxyRequest(c, "POST")
}

func getProxyHandler(c *gin.Context) {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "getProxyHandler", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	// Add cors headers for reply to original requester
	AddCorsHeaders(c)
	handleProxyRequest(c, "GET")
}

func IsBase64(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

func HandleFactoryInsight(c *gin.Context, request getProxyRequestPath, method string, bytes []byte) {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "HandleFactoryInsight", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	zap.S().Debugf("HandleFactoryInsight")

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
	zap.S().Debugf("proxyUrl: ", internal.SanitizeString(proxyUrl))
	zap.S().Debugf("path: ", internal.SanitizeString(path))

	// Validate proxy url
	u, err := url.Parse(fmt.Sprintf("%s%s%s", FactoryInsightBaseUrl, proxyUrl, path))

	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	DoProxiedRequest(c, err, u, "", authHeader, method, bytes)
}

// HandleFactoryInput handles proxy requests to factoryinput
func HandleFactoryInput(c *gin.Context, request getProxyRequestPath, method string, bodyBytes []byte) {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "HandleFactoryInput", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

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
		handleInvalidInputError(c, err)
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
	u, err := url.Parse(fmt.Sprintf("%sapi/v1/%s", FactoryInputBaseURL, proxyUrl))
	zap.S().Warnf("Proxified URL: %s", internal.SanitizeString(u.String()))
	if err != nil {
		zap.S().Warnf("url.Parse failed: %s", internal.SanitizeString(fmt.Sprintf("%sapi/v1/%s", FactoryInputBaseURL, proxyUrl)))
		handleInvalidInputError(c, err)
		return
	}

	// Split proxy url into customer, location, asset, value
	s := strings.Split(u.Path, "/")
	if len(s) != 7 {
		zap.S().Warnf("String split failed", len(s))
		handleInvalidInputError(c, errors.New(fmt.Sprintf("factoryinput url invalid: %d", len(s))))
		return
	}

	zap.S().Debugf("splits: ", internal.SanitizeStringArray(s))

	customer := s[3]

	zap.S().Warnf("GetOrgas")
	// Get grafana organizations of user
	orgas, err := user.GetOrgas(sessionCookie)
	if err != nil {
		zap.S().Warnf("GetOrgas failed %s, %s", err, internal.SanitizeString(sessionCookie))
		handleInvalidInputError(c, err)
		return
	}

	zap.S().Warnf("My Orgas: ", orgas)
	zap.S().Warnf("Customer: %s", internal.SanitizeString(customer))
	// Check if user is in orga, with same name as customer
	allowedOrg := false
	for _, orgsElement := range orgas {
		if orgsElement.Name == customer {
			zap.S().Warnf("%s vs %s", internal.SanitizeString(orgsElement.Name), internal.SanitizeString(customer))
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
	DoProxiedRequest(c, err, u, sessionCookie, ak, method, bodyBytes)
}

func DoProxiedRequest(c *gin.Context, err error, u *url.URL, sessionCookie string, authorizationKey string, method string, bodyBytes []byte) {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "DoProxiedRequest", oteltrace.WithAttributes(attribute.String("error", fmt.Sprintf("%s", err))))
		defer span.End()
	}
	// Proxy request to backend
	client := &http.Client{}

	zap.S().Debugf("Request URL: %s", internal.SanitizeString(u.String()))
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

		var req *http.Request
		if bodyBytes != nil && len(bodyBytes) > 0 {
			zap.S().Warnf("Request with body bytes: %s", internal.SanitizeByteArray(bodyBytes))
			req, err = http.NewRequest(method, u.String(), bytes.NewBuffer(bodyBytes))
		} else {
			zap.S().Warnf("Request without body bytes")
			req, err = http.NewRequest(method, u.String(), nil)

		}
		if err != nil {
			zap.S().Warnf("Request error: %s", err)
			return
		}
		// Add headers for backend
		if len(sessionCookie) > 0 {
			req.Header.Set("Cookie", fmt.Sprintf("grafana_session=%s", internal.SanitizeString(sessionCookie)))
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
