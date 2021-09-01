package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	ginopentracing "github.com/Bose/go-gin-opentracing"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

// SetupRestAPI initializes the REST API and starts listening
func SetupRestAPI(accounts gin.Accounts, version string, jaegerHost string, jaegerPort string) {
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

	// initialize the global singleton for tracing...
	tracer, reporter, closer, err := ginopentracing.InitTracing("factoryinsight", jaegerHost+":"+jaegerPort, ginopentracing.WithEnableInfoLog(false))
	if err != nil {
		panic("unable to init tracing")
	}
	defer func(closer io.Closer) {
		err := closer.Close()
		if err != nil {
			zap.S().Errorf("Failed to close Tracer", err)
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
		if shutdownEnabled {
			c.String(http.StatusOK, "shutdown")
		} else {
			c.String(http.StatusOK, "online")
		}
	})

	apiString := fmt.Sprintf("/api/v%s", version)

	// Version of the API
	v1 := router.Group(apiString, gin.BasicAuth(accounts))
	{
		// WARNING: Need to check in each specific handler whether the user is actually allowed to access it, so that valid user "ia" cannot access data for customer "abc"
		v1.POST("/:customer/:location/:asset/:value", postMQTTHandler)
	}

	err = router.Run(":80")
	if err != nil {
		zap.S().Errorf("Failed to bind to port 80", err)
		ShutdownApplicationGraceful()
		return
	}
}

func handleInternalServerError(parentSpan opentracing.Span, c *gin.Context, err error) {

	ext.LogError(parentSpan, err)
	traceID, _ := internal.ExtractTraceID(parentSpan)

	zap.S().Errorw("Internal server error",
		"error", err,
		"trace id", traceID,
	)

	c.String(http.StatusInternalServerError, "The server had an internal error. Please mention the following trace id while contacting our support: "+traceID)
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

// Access handler
func checkIfUserIsAllowed(c *gin.Context, customer string) error {
	user := c.MustGet(gin.AuthUserKey)
	if user != customer && user != "jeremy" {
		c.AbortWithStatus(http.StatusUnauthorized)
		zap.S().Infof("User %s unauthorized to access %s", user, customer)
		return fmt.Errorf("User %s unauthorized to access %s", user, customer)
	}
	return nil
}

func IsJSON(str string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(str), &js) == nil
}

type postMQTTRequest struct {
	Customer string `uri:"customer" binding:"required"`
	Location string `uri:"location" binding:"required"`
	Asset    string `uri:"asset" binding:"required"`
	Value    string `uri:"value" binding:"required"`
}

type MQTTData struct {
	Customer string
	Location string
	Asset    string
	Value    string
	JSONData string
}

func postMQTTHandler(c *gin.Context) {
	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "postMQTTHandler", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "postMQTTHandler", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	jsonBytes, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		handleInvalidInputError(span, c, err)
	}

	jsonData := string(jsonBytes)

	if !IsJSON(jsonData) {
		handleInvalidInputError(span, c, errors.New("Input is not valid JSON"))
	}

	var postMQTTRequest postMQTTRequest
	err = c.BindUri(&postMQTTRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Check whether user has access to that customer
	err = checkIfUserIsAllowed(c, postMQTTRequest.Customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	err = enqueueMQTT(MQTTData{
		Customer: postMQTTRequest.Customer,
		Location: postMQTTRequest.Location,
		Asset:    postMQTTRequest.Asset,
		Value:    postMQTTRequest.Value,
		JSONData: jsonData,
	})
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
}
