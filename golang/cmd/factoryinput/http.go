package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	stdout "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	_ "go.opentelemetry.io/otel/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"io/ioutil"
	"net/http"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("factoryinput-server")

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
	tp := initTracer()

	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			panic(fmt.Sprintf("Error shutting down tracer provider: %v", err))
		}
	}()
	// tell gin to use the middleware
	router.Use(otelgin.Middleware("factoryinput"))

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

	err := router.Run(":80")
	if err != nil {
		zap.S().Errorf("Failed to bind to port 80", err)
		ShutdownApplicationGraceful()
		return
	}
}

func handleInternalServerError(c *gin.Context, err error) {
	traceID := "Failed to get traceID"
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "handleInternalServerError", oteltrace.WithAttributes(attribute.String("error", fmt.Sprintf("%s", err))))
		defer span.End()
		traceID = span.SpanContext().SpanID().String()
	}

	zap.S().Errorw("Internal server error",
		"error", internal.SanitizeString(err.Error()),
		"trace id", traceID,
	)

	c.String(http.StatusInternalServerError, "The server had an internal error. Please mention the following trace id while contacting our support: "+traceID)
}

func handleInvalidInputError(c *gin.Context, err error) {

	traceID := "Failed to get traceID"
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "handleInvalidInputError", oteltrace.WithAttributes(attribute.String("error", fmt.Sprintf("%s", err))))
		defer span.End()
		traceID = span.SpanContext().SpanID().String()
	}

	zap.S().Errorw("Invalid input error",
		"error", err,
		"trace id", traceID,
	)

	c.String(400, "You have provided a wrong input. Please check your parameters and mention the following trace id while contacting our support: "+traceID)
}

// Access handler
func checkIfUserIsAllowed(c *gin.Context, customer string) error {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "checkIfUserIsAllowed", oteltrace.WithAttributes(attribute.String("customer", fmt.Sprintf("%s", customer))))
		defer span.End()
	}

	user := c.MustGet(gin.AuthUserKey)
	if user != customer {
		c.AbortWithStatus(http.StatusUnauthorized)
		zap.S().Infof("User %s unauthorized to access %s", user, internal.SanitizeString(customer))
		return fmt.Errorf("User %s unauthorized to access %s", user, internal.SanitizeString(customer))
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
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "postMQTTHandler", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	jsonBytes, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		handleInvalidInputError(c, err)
	}

	jsonData := string(jsonBytes)
	zap.S().Warnf("jsonData: %s", internal.SanitizeString(jsonData))

	if !IsJSON(jsonData) {
		handleInvalidInputError(c, errors.New("Input is not valid JSON"))
	}

	var postMQTTRequest postMQTTRequest
	err = c.BindUri(&postMQTTRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Check whether user has access to that customer
	err = checkIfUserIsAllowed(c, postMQTTRequest.Customer)
	if err != nil {
		handleInternalServerError(c, err)
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
		handleInternalServerError(c, err)
		return
	}
}
