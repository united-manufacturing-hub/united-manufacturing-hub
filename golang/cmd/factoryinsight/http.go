package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	ginopentracing "github.com/Bose/go-gin-opentracing"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
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
	defer closer.Close()
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

	apiString := fmt.Sprintf("/api/v%s", version)

	// Version of the API
	v1 := router.Group(apiString, gin.BasicAuth(accounts))
	{
		// WARNING: Need to check in each specific handler whether the user is actually allowed to access it, so that valid user "ia" cannot access data for customer "abc"
		v1.GET("/:customer", getLocationsHandler)
		v1.GET("/:customer/:location", getAssetsHandler)
		v1.GET("/:customer/:location/:asset", getValuesHandler)
		v1.GET("/:customer/:location/:asset/:value", getDataHandler)
	}

	router.Run(":80")
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

// ---------------------- getLocations ----------------------

type getLocationsRequest struct {
	Customer string `uri:"customer" binding:"required"`
}

func getLocationsHandler(c *gin.Context) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "getLocationsHandler", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "getLocationsHandler", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getLocationsRequest getLocationsRequest
	var err error
	var locations []string

	err = c.BindUri(&getLocationsRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Check whether user has access to that customer
	err = checkIfUserIsAllowed(c, getLocationsRequest.Customer)
	if err != nil {
		return
	}

	// Fetching from the database
	locations, err = GetLocations(span, getLocationsRequest.Customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	c.JSON(http.StatusOK, locations)
}

// ---------------------- getAssets ----------------------

type getAssetsRequest struct {
	Customer string `uri:"customer" binding:"required"`
	Location string `uri:"location" binding:"required"`
}

func getAssetsHandler(c *gin.Context) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "getAssetsHandler", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "getAssetsHandler", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getAssetsRequest getAssetsRequest
	var err error
	var assets []string

	err = c.BindUri(&getAssetsRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Check whether user has access to that customer
	err = checkIfUserIsAllowed(c, getAssetsRequest.Customer)
	if err != nil {
		return
	}

	// Fetching from the database
	assets, err = GetAssets(span, getAssetsRequest.Customer, getAssetsRequest.Location)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	c.JSON(http.StatusOK, assets)
}

// ---------------------- getValues ----------------------

type getValuesRequest struct {
	Customer string `uri:"customer" binding:"required"`
	Location string `uri:"location" binding:"required"`
	Asset    string `uri:"asset" binding:"required"`
}

func getValuesHandler(c *gin.Context) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "getValuesHandler", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "getValuesHandler", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getValuesRequest getValuesRequest

	err := c.BindUri(&getValuesRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Check whether user has access to that customer
	err = checkIfUserIsAllowed(c, getValuesRequest.Customer)
	if err != nil {
		return
	}

	var values []string
	values = append(values, "count")
	values = append(values, "state")
	values = append(values, "currentState")
	values = append(values, "recommendation")
	values = append(values, "timeRange")
	values = append(values, "aggregatedStates")
	values = append(values, "availability")
	values = append(values, "performance")
	values = append(values, "oee")
	values = append(values, "shifts")
	values = append(values, "productionSpeed")
	values = append(values, "stateHistogram")
	values = append(values, "factoryLocations")
	values = append(values, "averageCleaningTime")
	values = append(values, "averageChangeoverTime")
	values = append(values, "upcomingMaintenanceActivities")
	// values = append(values, "maintenanceComponents")
	values = append(values, "maintenanceActivities")
	values = append(values, "uniqueProducts")
	values = append(values, "orderTable")
	values = append(values, "orderTimeline")

	// Get from cache if possible
	var cacheHit bool
	processValues, cacheHit := internal.GetDistinctProcessValuesFromCache(getValuesRequest.Customer, getValuesRequest.Location, getValuesRequest.Asset)

	if !cacheHit { // data NOT found
		processValues, err = GetDistinctProcessValues(span, getValuesRequest.Customer, getValuesRequest.Location, getValuesRequest.Asset)
		if err != nil {
			handleInternalServerError(span, c, err)
			return
		}

		// Store to cache if not yet existing
		go internal.StoreDistinctProcessValuesToCache(getValuesRequest.Customer, getValuesRequest.Location, getValuesRequest.Asset, processValues)
		zap.S().Debugf("Stored DistinctProcessValues to cache")
	}

	values = append(values, processValues...)

	c.JSON(http.StatusOK, values)
}

// ---------------------- getData ----------------------

type getDataRequest struct {
	Customer string `uri:"customer" binding:"required"`
	Location string `uri:"location" binding:"required"`
	Asset    string `uri:"asset" binding:"required"`
	Value    string `uri:"value" binding:"required"`
}

func getDataHandler(c *gin.Context) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "getDataHandler", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "getDataHandler", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getDataRequest getDataRequest
	var err error

	err = c.BindUri(&getDataRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Check whether user has access to that customer
	err = checkIfUserIsAllowed(c, getDataRequest.Customer)
	if err != nil {
		return
	}

	switch getDataRequest.Value {
	case "state":
		processStatesRequest(c, getDataRequest)
	case "count":
		processCountsRequest(c, getDataRequest)
	case "currentState":
		processCurrentStateRequest(c, getDataRequest)
	case "recommendation":
		processRecommendationRequest(c, getDataRequest)
	case "aggregatedStates":
		processAggregatedStatesRequest(c, getDataRequest)
	case "timeRange":
		processTimeRangeRequest(c, getDataRequest)
	case "availability":
		processAvailabilityRequest(c, getDataRequest)
	case "performance":
		processPerformanceRequest(c, getDataRequest)
	case "oee":
		processOEERequest(c, getDataRequest)
	case "productionSpeed":
		processProductionSpeedRequest(c, getDataRequest)
	case "shifts":
		processShiftsRequest(c, getDataRequest)
	case "stateHistogram":
		processStateHistogramRequest(c, getDataRequest)
	case "factoryLocations":
		processFactoryLocationsRequest(c, getDataRequest)
	case "averageCleaningTime":
		processAverageCleaningTimeRequest(c, getDataRequest)
	case "averageChangeoverTime":
		processAverageChangeoverTimeRequest(c, getDataRequest)
	case "upcomingMaintenanceActivities":
		processUpcomingMaintenanceActivitiesRequest(c, getDataRequest)
	case "maintenanceComponents":
		processMaintenanceComponentsRequest(c, getDataRequest)
	case "maintenanceActivities":
		processMaintenanceActivitiesRequest(c, getDataRequest)
	case "uniqueProducts":
		processUniqueProductsRequest(c, getDataRequest)
	case "orderTable":
		processOrderTableRequest(c, getDataRequest)
	case "orderTimeline":
		processOrderTimelineRequest(c, getDataRequest)
	default:
		if strings.HasPrefix(getDataRequest.Value, "process_") {
			processProcessValueRequest(c, getDataRequest)
		} else {
			handleInvalidInputError(span, c, err)
			return
		}

	}

}

// ---------------------- getStates ----------------------

type getStatesRequest struct {
	From              time.Time `form:"from" binding:"required"`
	To                time.Time `form:"to" binding:"required"`
	KeepStatesInteger bool      `form:"keepStatesInteger"`
}

// processStatesRequest is responsible for fetching all required data and calculating states over time.
// The result is usually visualized in "DiscretePanel" in Grafana.
func processStatesRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### activate jaeger tracing ###
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processStatesRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processStatesRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getStatesRequest getStatesRequest
	var err error

	err = c.BindQuery(&getStatesRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	from := getStatesRequest.From
	to := getStatesRequest.To
	keepStatesInteger := getStatesRequest.KeepStatesInteger

	// ### jaeger addon ###
	span.SetTag("customer", customer)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)
	span.SetTag("keepStatesInteger", keepStatesInteger)

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(span, customer, location, asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// raw states from database
	rawStates, err := GetStatesRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := processStatesOptimized(span, assetID, rawStates, rawShifts, countSlice, orderArray, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "state"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: #90 Return timestamps in RFC3339 in /state

	// Loop through all datapoints
	for _, dataPoint := range processedStates {
		if keepStatesInteger {
			fullRow := []interface{}{dataPoint.State, float64(dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
			data.Datapoints = append(data.Datapoints, fullRow)
		} else {
			fullRow := []interface{}{ConvertStateToString(span, dataPoint.State, 0, configuration), float64(dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
			data.Datapoints = append(data.Datapoints, fullRow)
		}
	}

	c.JSON(http.StatusOK, data)
}

// ---------------------- getAggregatedStates ----------------------

type getAggregatedStatesRequest struct {
	From              time.Time `form:"from" binding:"required"`
	To                time.Time `form:"to" binding:"required"`
	IncludeRunning    *bool     `form:"includeRunning" binding:"required"` //*bool is required, see also https://github.com/gin-gonic/gin/issues/814
	KeepStatesInteger bool      `form:"keepStatesInteger"`
	AggregationType   int       `form:"aggregationType"`
}

// processAggregatedStatesRequest gets all states (including running). This can be used to calculate availability.
// If the aggregationType is 0 it will aggregate over the entire time span.
// If the aggregationType is not 0 it will aggregate over various categories, e.g. day or hour
func processAggregatedStatesRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### activate jaeger tracing ###
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processAggregatedStatesRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processAggregatedStatesRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###

	var getAggregatedStatesRequest getAggregatedStatesRequest
	var err error

	err = c.BindQuery(&getAggregatedStatesRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	from := getAggregatedStatesRequest.From
	to := getAggregatedStatesRequest.To
	keepStatesInteger := getAggregatedStatesRequest.KeepStatesInteger
	aggregationType := getAggregatedStatesRequest.AggregationType
	includeRunning := getAggregatedStatesRequest.IncludeRunning

	// ### jaeger addon ###
	span.SetTag("customer", customer)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)
	span.SetTag("keepStatesInteger", keepStatesInteger)
	span.SetTag("aggregationType", aggregationType)
	span.SetTag("includeRunning", includeRunning)

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(span, customer, location, asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	// TODO: parallelize

	// raw states from database
	rawStates, err := GetStatesRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	processedStates, err := processStatesOptimized(span, assetID, rawStates, rawShifts, countSlice, orderArray, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// TODO: #84 Convert states to string when keepStatesInteger is false and aggregationType is 1

	// Prepare JSON
	var data datamodel.DataResponseAny
	if aggregationType == 0 { // default case. aggregate over everything
		data.ColumnNames = []string{"state", "duration"}

		data.Datapoints, err = CalculateStopParetos(span, processedStates, from, to, *includeRunning, keepStatesInteger, configuration)

		if err != nil {
			handleInternalServerError(span, c, err)
			return
		}
	} else {
		data.ColumnNames = []string{"category", "state", "duration"}

		if aggregationType == 1 { // category: hour in a day

			// create resultDatapoints [][]float64. resultDatapoints[HOUR][STATE] = sum of STATE in that hour
			var resultDatapoints [24][datamodel.MaxState]float64 //24 hours in a day, 2000 different states (0 - 1999)

			// round up "from" till the next full hour
			tempFrom := time.Date(from.Year(), from.Month(), from.Day(), from.Hour()+1, 0, 0, 0, from.Location())

			if !tempFrom.Before(to) {
				zap.S().Warnf("Not big enough time range (!tempFrom.Before(to))", tempFrom, to)
			}

			// round down "to" till the next full hour
			tempTo := time.Date(to.Year(), to.Month(), to.Day(), to.Hour(), 0, 0, 0, to.Location())

			if !tempTo.After(from) {
				zap.S().Warnf("Not big enough time range (!tempTo.After(from))", tempTo, from)
			}

			// Call CalculateStopParetos for every hour between "from" and "to" and add results to resultDatapoints
			oldD := tempFrom

			for d := tempFrom; d.After(tempTo) == false; d = d.Add(time.Hour) { //timestamp is beginning of the state. d is current progress.
				if d == oldD { //if first entry
					continue
				}

				currentHour := d.Hour()

				processedStatesCleaned := removeUnnecessaryElementsFromStateSlice(processedStates, oldD, d)

				tempResult, err := CalculateStopParetos(span, processedStatesCleaned, oldD, d, *includeRunning, true, configuration)
				if err != nil {
					handleInternalServerError(span, c, err)
					return
				}

				for _, dataPoint := range tempResult {
					state := dataPoint[0].(int)
					duration := dataPoint[1].(float64)

					resultDatapoints[currentHour][state] += duration
				}

				oldD = d
			}

			// create return JSON
			for index, currentHourDatapoint := range resultDatapoints {
				hour := index

				for state, duration := range currentHourDatapoint {

					if duration > 0 {
						fullRow := []interface{}{hour, state, duration}
						data.Datapoints = append(data.Datapoints, fullRow)
					}

				}

			}

		}
	}

	c.JSON(http.StatusOK, data)
}

// ---------------------- getAvailability ----------------------

type getAvailabilityRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

func processAvailabilityRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### activate jaeger tracing ###
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processAvailabilityRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processAvailabilityRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getAvailabilityRequest getAvailabilityRequest
	var err error

	err = c.BindQuery(&getAvailabilityRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	from := getAvailabilityRequest.From
	to := getAvailabilityRequest.To

	// ### jaeger addon ###
	span.SetTag("customer", customer)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(span, customer, location, asset)
	if err != nil {
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := processStatesOptimized(span, assetID, rawStates, rawShifts, countSlice, orderArray, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "availability"
	data.ColumnNames = []string{JSONColumnName}

	data.Datapoints, err = CalculateAvailability(span, processedStates, from, to, configuration)

	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

// ---------------------- getPerformance ----------------------

type getPerformanceRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

func processPerformanceRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### activate jaeger tracing ###
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processPerformanceRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processPerformanceRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getPerformanceRequest getPerformanceRequest
	var err error

	err = c.BindQuery(&getPerformanceRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	from := getPerformanceRequest.From
	to := getPerformanceRequest.To

	// ### jaeger addon ###
	span.SetTag("customer", customer)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(span, customer, location, asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := processStatesOptimized(span, assetID, rawStates, rawShifts, countSlice, orderArray, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "performance"
	data.ColumnNames = []string{JSONColumnName}

	data.Datapoints, err = CalculatePerformance(span, processedStates, from, to, configuration)

	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

// ---------------------- getOEE ----------------------

type getOEERequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

func processOEERequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### activate jaeger tracing ###
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processOEERequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processOEERequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getOEERequest getOEERequest
	var err error

	err = c.BindQuery(&getOEERequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	from := getOEERequest.From
	to := getOEERequest.To

	// ### jaeger addon ###
	span.SetTag("customer", customer)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(span, customer, location, asset)
	if err != nil {
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "oee"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: #85 Ensure that multi-day OEE is split up during multiples 00:00 instead of multiples of the from time.

	// TODO: create JSON and calculate in the same paragraph
	for current := from; current != to; {
		var tempDatapoints []interface{}

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value

			processedStates, err := processStates(span, assetID, rawStates, rawShifts, countSlice, orderArray, current, to, configuration)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			tempDatapoints, err = CalculateOEE(span, processedStates, current, to, configuration)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			current = to
		} else { //otherwise, calculate for entire time range

			processedStates, err := processStates(span, assetID, rawStates, rawShifts, countSlice, orderArray, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			tempDatapoints, err = CalculateOEE(span, processedStates, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			current = currentTo
		}
		// only add it if there is a valid datapoint. do not add areas with no state times
		if tempDatapoints != nil {
			data.Datapoints = append(data.Datapoints, tempDatapoints)
		}
	}

	c.JSON(http.StatusOK, data)
}

// ---------------------- getStateHistogram ----------------------

type getStateHistogramRequest struct {
	From              time.Time `form:"from" binding:"required"`
	To                time.Time `form:"to" binding:"required"`
	IncludeRunning    bool      `form:"includeRunning"`
	KeepStatesInteger bool      `form:"keepStatesInteger"`
}

func processStateHistogramRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### activate jaeger tracing ###
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processStateHistogramRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processStateHistogramRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getStateHistogramRequest getStateHistogramRequest
	var err error

	err = c.BindQuery(&getStateHistogramRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	from := getStateHistogramRequest.From
	to := getStateHistogramRequest.To
	includeRunning := getStateHistogramRequest.IncludeRunning
	keepStatesInteger := getStateHistogramRequest.KeepStatesInteger

	// ### jaeger addon ###
	span.SetTag("customer", customer)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)
	span.SetTag("includeRunning", includeRunning)
	span.SetTag("keepStatesInteger", keepStatesInteger)

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(span, customer, location, asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := processStatesOptimized(span, assetID, rawStates, rawShifts, countSlice, orderArray, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### create JSON ###
	var data datamodel.DataResponseAny
	data.ColumnNames = []string{"state", "occurances"}

	data.Datapoints, err = CalculateStateHistogram(span, processedStates, from, to, includeRunning, keepStatesInteger, configuration)

	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

// -----------------------------------------------------

type getCurrentStateRequest struct {
	KeepStatesInteger bool `form:"keepStatesInteger"`
}

type getShiftsRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type getUniqueProductsRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type getProcessValueRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type getOrderRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type getCountsRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type getproductionSpeedRequest struct {
	From                time.Time `form:"from" binding:"required"`
	To                  time.Time `form:"to" binding:"required"`
	AggregationInterval int       `form:"aggregationInterval"`
}

func processCurrentStateRequest(c *gin.Context, getDataRequest getDataRequest) {
	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processCurrentStateRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processCurrentStateRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getCurrentStateRequest getCurrentStateRequest
	var err error

	err = c.BindQuery(&getCurrentStateRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Fetching from the database
	// TODO: #89 Return timestamps in RFC3339 in /currentState
	state, err := GetCurrentState(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getCurrentStateRequest.KeepStatesInteger)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func processCountsRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processCountsRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processCountsRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getCountsRequest getCountsRequest
	var err error
	var counts datamodel.DataResponseAny

	err = c.BindQuery(&getCountsRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Fetching from the database
	// TODO: #88 Return timestamps in RFC3339 in /counts
	counts, err = GetCounts(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getCountsRequest.From, getCountsRequest.To)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

func processRecommendationRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processRecommendationRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processRecommendationRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// Fetching from the database
	recommendations, err := GetRecommendations(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	c.JSON(http.StatusOK, recommendations)
}

func processShiftsRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processShiftsRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processShiftsRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getShiftsRequest getShiftsRequest
	var err error

	err = c.BindQuery(&getShiftsRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Fetching from the database
	shifts, err := GetShifts(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getShiftsRequest.From, getShiftsRequest.To)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	c.JSON(http.StatusOK, shifts)
}

func processProcessValueRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processProcessValueRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processProcessValueRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getProcessValueRequest getProcessValueRequest
	var err error

	err = c.BindQuery(&getProcessValueRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	valueName := strings.TrimPrefix(getDataRequest.Value, "process_")

	// TODO: #96 Return timestamps in RFC3339 in /processValue

	// Fetching from the database
	processValues, err := GetProcessValue(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getProcessValueRequest.From, getProcessValueRequest.To, valueName)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	c.JSON(http.StatusOK, processValues)
}

func processTimeRangeRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processTimeRangeRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processTimeRangeRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// Fetching from the database
	timeRange, err := GetDataTimeRangeForAsset(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	c.JSON(http.StatusOK, timeRange)
}

func processUpcomingMaintenanceActivitiesRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processUpcomingMaintenanceActivitiesRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processUpcomingMaintenanceActivitiesRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	rawData, err := GetUpcomingTimeBasedMaintenanceActivities(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	var data datamodel.DataResponseAny
	data.ColumnNames = []string{"Machine", "Component", "Activity", "Duration", "Status"}

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, getDataRequest.Customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// TODO: #100 Return timestamps in RFC3339 in /maintenanceActivities

	for _, timeBasedMaintenanceActivity := range rawData {
		var activityString = ConvertActivityToString(span, timeBasedMaintenanceActivity.ActivityType, configuration)

		if timeBasedMaintenanceActivity.DurationInDays.Valid != true || timeBasedMaintenanceActivity.LatestActivity.Valid != true || timeBasedMaintenanceActivity.NextActivity.Valid != true {
			fullRow := []interface{}{getDataRequest.Asset, timeBasedMaintenanceActivity.ComponentName, activityString, 0, 0}
			data.Datapoints = append(data.Datapoints, fullRow)
		} else {
			var status = 2                                                //green by default
			if timeBasedMaintenanceActivity.DurationInDays.Float64 <= 0 { // critical
				status = 0
			} else if timeBasedMaintenanceActivity.DurationInDays.Float64*24/float64(timeBasedMaintenanceActivity.IntervallInHours) < 0.3 { // under a third of the runtime we are on oragne area
				status = 1
			}

			fullRow := []interface{}{getDataRequest.Asset, timeBasedMaintenanceActivity.ComponentName, activityString, timeBasedMaintenanceActivity.DurationInDays.Float64, status}
			data.Datapoints = append(data.Datapoints, fullRow)
		}
	}

	c.JSON(http.StatusOK, data)
}

func processOrderTableRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processOrderTableRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processOrderTableRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getOrderRequest getOrderRequest
	var err error

	err = c.BindQuery(&getOrderRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Fetch data from database

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, getDataRequest.Customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	assetID, err := GetAssetID(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	rawOrders, err := GetOrdersRaw(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get counts for actual units calculation
	countSlice, err := GetCountsRaw(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// raw states from database
	rawStates, err := GetStatesRaw(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// TODO: #98 Return timestamps in RFC3339 in /orderTable

	// Process data
	data, err := calculateOrderInformation(span, rawOrders, countSlice, assetID, rawStates, rawShifts, configuration, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func processOrderTimelineRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processOrderTimelineRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processOrderTimelineRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getOrderRequest getOrderRequest
	var err error

	err = c.BindQuery(&getOrderRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// TODO: #97 Return timestamps in RFC3339 in /orderTimeline

	// Process data
	data, err := GetOrdersTimeline(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func processMaintenanceActivitiesRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processMaintenanceActivitiesRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processMaintenanceActivitiesRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// Fetching from the database
	data, err := GetMaintenanceActivities(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func processUniqueProductsRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processUniqueProductsRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processUniqueProductsRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getUniqueProductsRequest getUniqueProductsRequest
	var err error

	err = c.BindQuery(&getUniqueProductsRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// TODO: #99 Return timestamps in RFC3339 in /uniqueProducts

	// Fetching from the database
	uniqueProducts, err := GetUniqueProducts(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getUniqueProductsRequest.From, getUniqueProductsRequest.To)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	c.JSON(http.StatusOK, uniqueProducts)
}

func processMaintenanceComponentsRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processMaintenanceComponentsRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processMaintenanceComponentsRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// Fetching from the database
	assetID, err := GetAssetID(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	data, err := GetComponents(span, assetID)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func processProductionSpeedRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processProductionSpeedRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processProductionSpeedRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var getproductionSpeedRequest getproductionSpeedRequest
	var err error
	var counts datamodel.DataResponseAny

	err = c.BindQuery(&getproductionSpeedRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	// Fetching from the database
	counts, err = GetProductionSpeed(span, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getproductionSpeedRequest.From, getproductionSpeedRequest.To, getproductionSpeedRequest.AggregationInterval)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

func processFactoryLocationsRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Jaeger tracing
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processFactoryLocationsRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processFactoryLocationsRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	var data datamodel.DataResponseAny
	data.ColumnNames = []string{"Location", "Metric", "Geohash"}

	fullRow := []interface{}{"Aachen", 80, "u1h2fe"}
	data.Datapoints = append(data.Datapoints, fullRow)

	c.JSON(http.StatusOK, data)
}

// ---------------------- getAverageCleaningTime ----------------------

type getAverageCleaningTimeRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

func processAverageCleaningTimeRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### activate jaeger tracing ###
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processAverageCleaningTimeRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processAverageCleaningTimeRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getAverageCleaningTimeRequest getAverageCleaningTimeRequest
	var err error

	err = c.BindQuery(&getAverageCleaningTimeRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	from := getAverageCleaningTimeRequest.From
	to := getAverageCleaningTimeRequest.To

	// ### jaeger addon ###
	span.SetTag("customer", customer)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(span, customer, location, asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "averageCleaningTime"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: create JSON and calculate in the same paragraph
	for current := from; current != to; {
		var tempDatapoints []interface{}

		currentTo := current.AddDate(0, 0, 1)

		// TODO: #93 Rework /averageCleaningTime, /averageChangeovertime, CalculateAverageStateTime() to be compatible with new datamodel

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value

			processedStates, err := processStates(span, assetID, rawStates, rawShifts, countSlice, orderArray, current, to, configuration)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(span, processedStates, current, to, configuration, 18) // Cleaning is 18
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			current = to
		} else { //otherwise, calculate for entire time range

			processedStates, err := processStates(span, assetID, rawStates, rawShifts, countSlice, orderArray, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(span, processedStates, current, currentTo, configuration, 18)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			current = currentTo
		}

		// only add it if there is a valid datapoint. do not add areas with no state times
		if tempDatapoints != nil {
			data.Datapoints = append(data.Datapoints, tempDatapoints)
		}

	}

	c.JSON(http.StatusOK, data)
}

// ---------------------- getAverageChangeoverTime ----------------------

type getAverageChangeoverTimeRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

func processAverageChangeoverTimeRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### activate jaeger tracing ###
	var span opentracing.Span
	if cspan, ok := c.Get("tracing-context"); ok {
		span = ginopentracing.StartSpanWithParent(cspan.(opentracing.Span).Context(), "processAverageChangeoverTimeRequest", c.Request.Method, c.Request.URL.Path)
	} else {
		span = ginopentracing.StartSpanWithHeader(&c.Request.Header, "processAverageChangeoverTimeRequest", c.Request.Method, c.Request.URL.Path)
	}
	defer span.Finish()

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getAverageChangeoverTimeRequest getAverageChangeoverTimeRequest
	var err error

	err = c.BindQuery(&getAverageChangeoverTimeRequest)
	if err != nil {
		handleInvalidInputError(span, c, err)
		return
	}

	from := getAverageChangeoverTimeRequest.From
	to := getAverageChangeoverTimeRequest.To

	// ### jaeger addon ###
	span.SetTag("customer", customer)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(span, customer, location, asset)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(span, customer)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(span, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(span, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(span, c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "averageChangeoverTime"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: create JSON and calculate in the same paragraph
	for current := from; current != to; {
		var tempDatapoints []interface{}

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value

			processedStates, err := processStates(span, assetID, rawStates, rawShifts, countSlice, orderArray, current, to, configuration)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(span, processedStates, current, to, configuration, datamodel.ChangeoverState)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			current = to
		} else { //otherwise, calculate for entire time range

			processedStates, err := processStates(span, assetID, rawStates, rawShifts, countSlice, orderArray, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(span, processedStates, current, currentTo, configuration, datamodel.ChangeoverState)
			if err != nil {
				handleInternalServerError(span, c, err)
				return
			}

			current = currentTo
		}
		// only add it if there is a valid datapoint. do not add areas with no state times
		if tempDatapoints != nil {
			data.Datapoints = append(data.Datapoints, tempDatapoints)
		}
	}

	c.JSON(http.StatusOK, data)
}
