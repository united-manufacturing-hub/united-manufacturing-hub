package main

import (
	"fmt"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/docs"
	"net/http"
	"strings"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
)

// SetupRestAPI initializes the REST API and starts listening
// @Summary Initializes the REST API and starts listening
// @Description sets up REST API via Gin and uses ginzap for logging
// @ID setup-rest-api
func SetupRestAPI(accounts gin.Accounts, version string) {
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
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	router.Run(":80")
}

func handleInternalServerError(c *gin.Context, err error) {

	zap.S().Errorw("Internal server error",
		"error", err,
	)

	c.String(http.StatusInternalServerError, "The server had an internal error.")
}

func handleInvalidInputError(c *gin.Context, err error) {

	zap.S().Errorw("Invalid input error",
		"error", internal.SanitizeString(err.Error()),
	)

	c.String(400, "You have provided a wrong input. Please check your parameters.")
}

// Access handler
func checkIfUserIsAllowed(c *gin.Context, customer string) error {

	user := c.MustGet(gin.AuthUserKey)
	if user != customer {
		c.AbortWithStatus(http.StatusUnauthorized)
		zap.S().Infof("User %s unauthorized to access %s", user, internal.SanitizeString(customer))
		return fmt.Errorf("user %s unauthorized to access %s", user, internal.SanitizeString(customer))
	}
	return nil
}

// ---------------------- getLocations ----------------------

type getLocationsRequest struct {
	Customer string `uri:"customer" binding:"required"`
}

// getLocationsHandler
// @Summary Fetches the Location for the Message topic
// @Description Checks for User authorization, then reads out the location and returns it
// @ID get-locations
// @Accept json
// @Produce json
// @Param customer path string true "Name of customer whose locations are requested"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer} [GET]
func getLocationsHandler(c *gin.Context) {

	var getLocationsRequest getLocationsRequest
	var err error
	var locations []string

	err = c.BindUri(&getLocationsRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Check whether user has access to that customer
	err = checkIfUserIsAllowed(c, getLocationsRequest.Customer)
	if err != nil {
		return
	}

	// Fetching from the database
	locations, err = GetLocations(c, getLocationsRequest.Customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, locations)
}

// ---------------------- getAssets ----------------------

type getAssetsRequest struct {
	Customer string `uri:"customer" binding:"required"`
	Location string `uri:"location" binding:"required"`
}

// getAssetsHandler
// @Summary Fetches the assets for the message topic
// @Description Checks for User authorization, then reads out the assets and returns them
// @ID get-assets
// @Accept json
// @Produce json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose assets are requested"
// @Success 200	{array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/ [GET]
func getAssetsHandler(c *gin.Context) {

	var getAssetsRequest getAssetsRequest
	var err error
	var assets []string

	err = c.BindUri(&getAssetsRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Check whether user has access to that customer
	err = checkIfUserIsAllowed(c, getAssetsRequest.Customer)
	if err != nil {
		return
	}

	// Fetching from the database
	assets, err = GetAssets(c, getAssetsRequest.Customer, getAssetsRequest.Location)
	if err != nil {
		handleInternalServerError(c, err)
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

// getValuesHandler
// @Summary Fetches the values for a certain asset
// @Description Checks for User authorization, then reads out the values and returns them in an array of strings
// @ID get-values
// @Accept json
// @Produce json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose values are requested"
// @Success 200	{array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset} [GET]
func getValuesHandler(c *gin.Context) {

	var getValuesRequest getValuesRequest

	err := c.BindUri(&getValuesRequest)
	if err != nil {
		handleInvalidInputError(c, err)
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
	values = append(values, "quality")
	values = append(values, "oee")
	values = append(values, "shifts")
	values = append(values, "productionSpeed")
	values = append(values, "qualityRate")
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
	values = append(values, "uniqueProductsWithTags")
	values = append(values, "accumulatedProducts")
	values = append(values, "unstartedOrderTable")
	// Get from cache if possible
	var cacheHit bool
	processValues, cacheHit := internal.GetDistinctProcessValuesFromCache(getValuesRequest.Customer, getValuesRequest.Location, getValuesRequest.Asset)

	if !cacheHit { // data NOT found
		processValues, err = GetDistinctProcessValues(c, getValuesRequest.Customer, getValuesRequest.Location, getValuesRequest.Asset)
		if err != nil {
			handleInternalServerError(c, err)
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

//getDataHandler
// @Summary Fetches the data of the requested value
// @ID get-data
// @Accept json
// @Produce json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param value path string true "Name of value whose data is requested"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/{value} [GET]
func getDataHandler(c *gin.Context) {

	var getDataRequest getDataRequest
	var err error

	err = c.BindUri(&getDataRequest)
	if err != nil {
		handleInvalidInputError(c, err)
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
	case "quality":
		processQualityRequest(c, getDataRequest)
	case "oee":
		processOEERequest(c, getDataRequest)
	case "productionSpeed":
		processProductionSpeedRequest(c, getDataRequest)
	case "qualityRate":
		processQualityRateRequest(c, getDataRequest)
	case "shifts":
		processShiftsRequest(c, getDataRequest)
	case "stateHistogram":
		processStateHistogramRequest(c, getDataRequest)
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
	case "uniqueProductsWithTags":
		processUniqueProductsWithTagsRequest(c, getDataRequest)
	case "accumulatedProducts":
		processAccumulatedProducts(c, getDataRequest)
	case "unstartedOrderTable":
		processUnstartedOrderTableRequest(c, getDataRequest)
	default:
		if strings.HasPrefix(getDataRequest.Value, "process_") {
			processProcessValueRequest(c, getDataRequest)
		} else {
			handleInvalidInputError(c, err)
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

//processStatesRequest
// @Summary processes data for state Requests from one time-point to another of the asset
// @ID process-states-request
// @Accept json
// @Produce json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "Time point of the start of the time frame"
// @Param to query int true "Time point of the end of the time frame"
// @Param keepStatesInteger	query bool false "Collects states as integer codes if true and as string if false"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/state [GET]
func processStatesRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getStatesRequest getStatesRequest
	var err error

	err = c.BindQuery(&getStatesRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	from := getStatesRequest.From
	to := getStatesRequest.To
	keepStatesInteger := getStatesRequest.KeepStatesInteger

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(c, customer, location, asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(c, customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// raw states from database
	rawStates, err := GetStatesRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := processStatesOptimized(c, assetID, rawStates, rawShifts, countSlice, orderArray, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
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
			fullRow := []interface{}{ConvertStateToString(c, dataPoint.State, configuration), float64(dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
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

//processAggregatedStatesRequest
// @Summary Gets all states, can be used to calculate availability.
// @ID process-aggregated-states-request
// @Accept json
// @Produce json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "Time point of the start of the time frame"
// @Param to query int true "Time point of the end of the time frame"
// @Param includeRunning query bool true "Includes running states if true"
// @Param keepStatesInteger	query bool false "Collects states as integer codes if true and as string if false"
// @Param aggregationType query int false "Aggregationtype: 0 for entire time span, 1 for hours in days"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/aggregatedStates [GET]
func processAggregatedStatesRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###

	var getAggregatedStatesRequest getAggregatedStatesRequest
	var err error

	err = c.BindQuery(&getAggregatedStatesRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	from := getAggregatedStatesRequest.From
	to := getAggregatedStatesRequest.To
	keepStatesInteger := getAggregatedStatesRequest.KeepStatesInteger
	aggregationType := getAggregatedStatesRequest.AggregationType
	includeRunning := getAggregatedStatesRequest.IncludeRunning

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(c, customer, location, asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(c, customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	// TODO: parallelize

	// raw states from database
	rawStates, err := GetStatesRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	processedStates, err := processStatesOptimized(c, assetID, rawStates, rawShifts, countSlice, orderArray, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// TODO: #84 Convert states to string when keepStatesInteger is false and aggregationType is 1

	// Prepare JSON
	var data datamodel.DataResponseAny
	if aggregationType == 0 { // default case. aggregate over everything
		data.ColumnNames = []string{"state", "duration"}

		data.Datapoints, err = CalculateStopParetos(c, processedStates, to, *includeRunning, keepStatesInteger, configuration)

		if err != nil {
			handleInternalServerError(c, err)
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

			for d := tempFrom; !d.After(tempTo); d = d.Add(time.Hour) { //timestamp is beginning of the state. d is current progress.
				if d == oldD { //if first entry
					continue
				}

				currentHour := d.Hour()

				processedStatesCleaned := removeUnnecessaryElementsFromStateSlice(processedStates, oldD, d)

				tempResult, err := CalculateStopParetos(c, processedStatesCleaned, d, *includeRunning, true, configuration)
				if err != nil {
					handleInternalServerError(c, err)
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

//processAvailabilityRequest
// @Summary Returns availability in the given time frame as oee values for every day of the time frame
// @ID process-availability-request
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/availability [GET]
func processAvailabilityRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getAvailabilityRequest getAvailabilityRequest
	var err error

	err = c.BindQuery(&getAvailabilityRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	from := getAvailabilityRequest.From
	to := getAvailabilityRequest.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(c, customer, location, asset)
	if err != nil {
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(c, customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "oee"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: #85 Ensure that multi-day OEE is split up during multiples 00:00 instead of multiples of the from time.

	for current := from; current != to; {
		var tempDatapoints []interface{}

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, to, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAvailability(c, processedStates, current, to, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			current = to
		} else { //otherwise, calculate for entire time range

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAvailability(c, processedStates, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(c, err)
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

// ---------------------- getPerformance ----------------------

type getPerformanceRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

//processPerformanceRequest
// @Summary calculates the pareto performances for a given timeframe
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/performance [GET]
func processPerformanceRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getPerformanceRequest getPerformanceRequest
	var err error

	err = c.BindQuery(&getPerformanceRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	from := getPerformanceRequest.From
	to := getPerformanceRequest.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(c, customer, location, asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(c, customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "oee"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: #85 Ensure that multi-day OEE is split up during multiples 00:00 instead of multiples of the from time.

	for current := from; current != to; {
		var tempDatapoints []interface{}

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, to, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculatePerformance(c, processedStates, current, to, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			current = to
		} else { //otherwise, calculate for entire time range

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculatePerformance(c, processedStates, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(c, err)
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

// ---------------------- getQuality ----------------------

type getQualityRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

//processQualityRequest
// @Summary calculates Quality metric for OEE calculation
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/quality [GET]
func processQualityRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getQualityRequest getQualityRequest
	var err error

	err = c.BindQuery(&getQualityRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	from := getQualityRequest.From
	to := getQualityRequest.To

	// ### fetch necessary data from database ###

	// customer configuration
	_, err = GetCustomerConfiguration(c, customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "quality"
	data.ColumnNames = []string{JSONColumnName}

	// TODO: #85 Ensure that multi-day OEE is split up during multiples 00:00 instead of multiples of the from time.

	// TODO: create JSON and calculate in the same paragraph
	for current := from; current != to; {
		var tempDatapoints []interface{}

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value
			// split up countslice that it contains only counts between current and to
			countSliceSplit := SplitCountSlice(countSlice, current, to)

			// calculatequality(c,countslice)
			tempDatapoints, err = CalculateQuality(c, countSliceSplit)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			current = to
		} else { //otherwise, calculate for entire time range
			// split up countslice that it contains only counts between current and to
			countSliceSplit := SplitCountSlice(countSlice, current, currentTo)

			// calculatequality(c,countslice)
			tempDatapoints, err = CalculateQuality(c, countSliceSplit)
			if err != nil {
				handleInternalServerError(c, err)
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

// ---------------------- getOEE ----------------------

type getOEERequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

//processOEERequest
// @Summary calculates daily OEE values for asset in the given time frame
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/oee [GET]
func processOEERequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getOEERequest getOEERequest
	var err error

	err = c.BindQuery(&getOEERequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	from := getOEERequest.From
	to := getOEERequest.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(c, customer, location, asset)
	if err != nil {
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(c, customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
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

			countSliceSplit := SplitCountSlice(countSlice, current, to)

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, to, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateOEE(c, processedStates, countSliceSplit, current, to, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			current = to
		} else { //otherwise, calculate for entire time range

			countSliceSplit := SplitCountSlice(countSlice, current, currentTo)

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateOEE(c, processedStates, countSliceSplit, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(c, err)
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

//processStateHistogramRequest
// @Summary calculates a state histogram for a data state entry
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Param includeRunning query bool false "true if it should include running"
// @Param keepStatesInteger query bool false "true if you want to keep states as integers, false if you want them as strings"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/stateHistogram [GET]
func processStateHistogramRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getStateHistogramRequest getStateHistogramRequest
	var err error

	err = c.BindQuery(&getStateHistogramRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	from := getStateHistogramRequest.From
	to := getStateHistogramRequest.To
	includeRunning := getStateHistogramRequest.IncludeRunning
	keepStatesInteger := getStateHistogramRequest.KeepStatesInteger

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(c, customer, location, asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(c, customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := processStatesOptimized(c, assetID, rawStates, rawShifts, countSlice, orderArray, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// ### create JSON ###
	var data datamodel.DataResponseAny
	data.ColumnNames = []string{"state", "occurances"}

	data.Datapoints, err = CalculateStateHistogram(c, processedStates, includeRunning, keepStatesInteger, configuration)

	if err != nil {
		handleInternalServerError(c, err)
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

type getProductionSpeedRequest struct {
	From                time.Time `form:"from" binding:"required"`
	To                  time.Time `form:"to" binding:"required"`
	AggregationInterval int       `form:"aggregationInterval"`
}

type getQualityRateRequest struct {
	From                time.Time `form:"from" binding:"required"`
	To                  time.Time `form:"to" binding:"required"`
	AggregationInterval int       `form:"aggregationInterval"`
}

type getUniqueProductsWithTagsRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

//processCurrentStateRequest
// @Summary fetches current state of the asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param keepStatesInteger query string false "true if you want to keep states as integers, false if you want them as strings"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/currentState [GET]
func processCurrentStateRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getCurrentStateRequest getCurrentStateRequest
	var err error

	err = c.BindQuery(&getCurrentStateRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	// TODO: #89 Return timestamps in RFC3339 in /currentState
	state, err := GetCurrentState(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getCurrentStateRequest.KeepStatesInteger)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

//processCountRequest
// @Summary fetches count of the asset within the time frame
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/count [GET]
func processCountsRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getCountsRequest getCountsRequest
	var err error
	var counts datamodel.DataResponseAny

	err = c.BindQuery(&getCountsRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	// TODO: #88 Return timestamps in RFC3339 in /counts
	counts, err = GetCounts(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getCountsRequest.From, getCountsRequest.To)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

//processRecommendationRequest
// @Summary Gets action recommendations for the asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/recommendation [GET]
func processRecommendationRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Fetching from the database
	recommendations, err := GetRecommendations(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, recommendations)
}

//processShiftsRequest
// @Summary Gets action recommendations for the asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/shifts [GET]
func processShiftsRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getShiftsRequest getShiftsRequest
	var err error

	err = c.BindQuery(&getShiftsRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	shifts, err := GetShifts(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getShiftsRequest.From, getShiftsRequest.To)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, shifts)
}

//processProcessValueRequest
// @Summary Fetches custom process values from the asset within the timeframe
// @Description The values needs to be named with the prefix "process_"
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param value path string true "Name of the value, needs to start with `process_`"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/{value} [GET]
func processProcessValueRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getProcessValueRequest getProcessValueRequest
	var err error

	err = c.BindQuery(&getProcessValueRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	valueName := strings.TrimPrefix(getDataRequest.Value, "process_")

	// TODO: #96 Return timestamps in RFC3339 in /processValue

	// Fetching from the database
	processValues, err := GetProcessValue(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getProcessValueRequest.From, getProcessValueRequest.To, valueName)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, processValues)
}

//processTimeRangeRequest
// @Summary Fetches data from first time stamp to last one, e.g. for recommendations
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/timeRange [GET]
func processTimeRangeRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Fetching from the database
	timeRange, err := GetDataTimeRangeForAsset(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, timeRange)
}

//processUpcomingMaintenanceActivitiesRequest
// @Summary Fetches data about upcoming maintenance activities of the asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/upcomingMaintenanceActivities [GET]
func processUpcomingMaintenanceActivitiesRequest(c *gin.Context, getDataRequest getDataRequest) {

	rawData, err := GetUpcomingTimeBasedMaintenanceActivities(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	var data datamodel.DataResponseAny
	data.ColumnNames = []string{"Machine", "Component", "Activity", "Duration", "Status"}

	// customer configuration
	configuration, err := GetCustomerConfiguration(c, getDataRequest.Customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// TODO: #100 Return timestamps in RFC3339 in /maintenanceActivities

	for _, timeBasedMaintenanceActivity := range rawData {
		var activityString = ConvertActivityToString(c, timeBasedMaintenanceActivity.ActivityType, configuration)

		if !timeBasedMaintenanceActivity.DurationInDays.Valid || !timeBasedMaintenanceActivity.LatestActivity.Valid || !timeBasedMaintenanceActivity.NextActivity.Valid {
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

//processUnstartedOrderTableRequest
// @Summary Fetches unstarted orders of the asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/unstartedOrderTable [GET]
func processUnstartedOrderTableRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getOrderRequest getOrderRequest
	var err error

	err = c.BindQuery(&getOrderRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Fetch data from database
	zap.S().Debugf("Fetching order table for customer %s, location %s, asset %s, value: %v", getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getDataRequest.Value)

	zap.S().Debugf("GetUnstartedOrdersRaw")
	rawOrders, err := GetUnstartedOrdersRaw(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	data := datamodel.DataResponseAny{}
	data.ColumnNames = []string{"OrderName", "ProductName", "TargetUnits", "TimePerUnitInSeconds"}
	for _, order := range rawOrders {
		data.Datapoints = append(data.Datapoints, []interface{}{order.OrderName, order.ProductName, order.TargetUnits, order.TimePerUnitInSeconds})
	}

	c.JSON(http.StatusOK, data)
}

//processOrderTableRequest
// @Summary Fetches order data of the asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/orderTable [GET]
func processOrderTableRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getOrderRequest getOrderRequest
	var err error

	err = c.BindQuery(&getOrderRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Fetch data from database
	zap.S().Debugf("Fetching order table for customer %s, location %s, asset %s, value: %v", getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getDataRequest.Value)

	// customer configuration
	zap.S().Debugf("GetCustomerConfiguration")
	configuration, err := GetCustomerConfiguration(c, getDataRequest.Customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	zap.S().Debugf("GetAssetID")
	assetID, err := GetAssetID(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	zap.S().Debugf("GetOrdersRaw")
	rawOrders, err := GetOrdersRaw(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for actual units calculation
	zap.S().Debugf("GetCountsRaw")
	countSlice, err := GetCountsRaw(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// raw states from database
	zap.S().Debugf("GetStatesRaw")
	rawStates, err := GetStatesRaw(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	zap.S().Debugf("GetShiftsRaw")
	rawShifts, err := GetShiftsRaw(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// TODO: #98 Return timestamps in RFC3339 in /orderTable

	// Process data
	//zap.S().Debugf("calculateOrderInformation: rawOrders: %v, countSlice: %v, assetID: %v, rawStates: %v, rawShifts: %v, configuration: %v, Location: %v, Asset: %v", rawOrders, countSlice, assetID, rawStates, rawShifts, configuration, getDataRequest.Location, getDataRequest.Asset)
	data, err := calculateOrderInformation(c, rawOrders, countSlice, assetID, rawStates, rawShifts, configuration, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

//processOrderTimelineRequest
// @Summary Fetches all orders in a timeline in the time frame of the asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/orderTimeline [GET]
func processOrderTimelineRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getOrderRequest getOrderRequest
	var err error

	err = c.BindQuery(&getOrderRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// TODO: #97 Return timestamps in RFC3339 in /orderTimeline

	// Process data
	data, err := GetOrdersTimeline(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getOrderRequest.From, getOrderRequest.To)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

//processMaintenanceActivitiesRequest
// @Summary Gets all maintenance activities for an asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/maintenanceActivities [GET]
func processMaintenanceActivitiesRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Fetching from the database
	data, err := GetMaintenanceActivities(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

//processUniqueProductsRequest
// @Summary Gets all maintenance activities for an asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/uniqueProducts [GET]
func processUniqueProductsRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getUniqueProductsRequest getUniqueProductsRequest
	var err error

	err = c.BindQuery(&getUniqueProductsRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// TODO: #99 Return timestamps in RFC3339 in /uniqueProducts

	// Fetching from the database
	uniqueProducts, err := GetUniqueProducts(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getUniqueProductsRequest.From, getUniqueProductsRequest.To)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, uniqueProducts)
}

//processMaintenanceComponentsRequest
// @Summary Fetches all components for an asset
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/maintenanceComponents [GET]
func processMaintenanceComponentsRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Fetching from the database
	assetID, err := GetAssetID(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	data, err := GetComponents(c, assetID)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

//processProductionSpeedRequest
// @Summary Fetches production speed in a selected interval in minutes
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Param aggregationInterval query int false "interval in minutes"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/productionSpeed [GET]
func processProductionSpeedRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getProductionSpeedRequest getProductionSpeedRequest
	var err error
	var counts datamodel.DataResponseAny

	err = c.BindQuery(&getProductionSpeedRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	counts, err = GetProductionSpeed(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getProductionSpeedRequest.From, getProductionSpeedRequest.To, getProductionSpeedRequest.AggregationInterval)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

//processQualityRateRequest
// @Summary Fetches quality rate in a selected interval in minutes
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Param aggregationInterval query int false "interval in minutes"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/qualityRate [GET]
func processQualityRateRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getQualityRateRequest getQualityRateRequest
	var err error
	var counts datamodel.DataResponseAny

	err = c.BindQuery(&getQualityRateRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	counts, err = GetQualityRate(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getQualityRateRequest.From, getQualityRateRequest.To, getQualityRateRequest.AggregationInterval)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

// ---------------------- getAverageCleaningTime ----------------------

type getAverageCleaningTimeRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

//processAverageCleaningTimeRequest
// @Summary calculates the average cleaning time per day in the given time frame
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/averageCleaningTime [GET]
func processAverageCleaningTimeRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getAverageCleaningTimeRequest getAverageCleaningTimeRequest
	var err error

	err = c.BindQuery(&getAverageCleaningTimeRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	from := getAverageCleaningTimeRequest.From
	to := getAverageCleaningTimeRequest.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(c, customer, location, asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(c, customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
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

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, to, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(c, processedStates, current, to, configuration, 18) // Cleaning is 18
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			current = to
		} else { //otherwise, calculate for entire time range

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(c, processedStates, current, currentTo, configuration, 18)
			if err != nil {
				handleInternalServerError(c, err)
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

//processAverageChangeoverTimeRequest
// @Summary calculates the average changeover time per day in the given time frame
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/averageChangeoverTime [GET]
func processAverageChangeoverTimeRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getAverageChangeoverTimeRequest getAverageChangeoverTimeRequest
	var err error

	err = c.BindQuery(&getAverageChangeoverTimeRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	from := getAverageChangeoverTimeRequest.From
	to := getAverageChangeoverTimeRequest.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(c, customer, location, asset)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(c, customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(c, customer, location, asset, from, to, configuration)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(c, customer, location, asset, from, to)
	if err != nil {
		handleInternalServerError(c, err)
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

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, to, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(c, processedStates, current, to, configuration, datamodel.ChangeoverState)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			current = to
		} else { //otherwise, calculate for entire time range

			processedStates, err := processStates(c, assetID, rawStates, rawShifts, countSlice, orderArray, current, currentTo, configuration)
			if err != nil {
				handleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(c, processedStates, current, currentTo, configuration, datamodel.ChangeoverState)
			if err != nil {
				handleInternalServerError(c, err)
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

//processUniqueProductsWithTagsRequest
// @Summary fetches all unique products with tags during the specified time frame
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/uniqueProductsWithTags [GET]
func processUniqueProductsWithTagsRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getUniqueProductsWithTagsRequest getUniqueProductsWithTagsRequest
	var err error

	err = c.BindQuery(&getUniqueProductsWithTagsRequest)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	uniqueProductsWithTags, err := GetUniqueProductsWithTags(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getUniqueProductsWithTagsRequest.From, getUniqueProductsWithTagsRequest.To)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, uniqueProductsWithTags)
}

type getProcessAccumulatedProducts struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

//processAccumulatedProducts
// @Summary calculates the accumulated count of all products of the asset in the specified time frame
// @Accepts json
// @Produces json
// @Param customer path string true "Name of customer whose location is accessed"
// @Param location path string true "Name of location whose asset is accessed"
// @Param asset path string true "Name of asset whose value is accessed"
// @Param from query int true "start of the given time frame"
// @Param to query int true "end of the given time frame"
// @Success 200 {array} string "ok"
// @Failure 400 {string} string "You have provided a wrong input. Please check your parameters."
// @Failure 500 {string} string "Internal Error"
// @Router /{customer}/{location}/{asset}/accumulatedProducts [GET]
func processAccumulatedProducts(c *gin.Context, getDataRequest getDataRequest) {

	var getProcessAccumulatedProducts getProcessAccumulatedProducts
	var err error

	err = c.BindQuery(&getProcessAccumulatedProducts)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	accumulatedProducts, err := GetAccumulatedProducts(c, getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset, getProcessAccumulatedProducts.From, getProcessAccumulatedProducts.To)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, accumulatedProducts)

}
