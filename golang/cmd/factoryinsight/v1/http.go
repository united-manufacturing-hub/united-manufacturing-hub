// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"net/http"
	"strings"
	"time"
)

// ---------------------- getLocations ----------------------

type getLocationsRequest struct {
	Customer string `uri:"customer" binding:"required"`
}

func GetLocationsHandler(c *gin.Context) {

	var getLocationsRequestInstance getLocationsRequest
	var err error
	var locations []string

	err = c.BindUri(&getLocationsRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Check whether user has access to that customer
	err = helpers.CheckIfUserIsAllowed(c, getLocationsRequestInstance.Customer)
	if err != nil {
		return
	}

	// Fetching from the database
	locations, err = GetLocations(getLocationsRequestInstance.Customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, locations)
}

// ---------------------- getAssets ----------------------

type getAssetsRequest struct {
	Customer string `uri:"customer" binding:"required"`
	Location string `uri:"location" binding:"required"`
}

func GetAssetsHandler(c *gin.Context) {

	var getAssetsRequestInstance getAssetsRequest
	var err error
	var assets []string

	err = c.BindUri(&getAssetsRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Check whether user has access to that customer
	err = helpers.CheckIfUserIsAllowed(c, getAssetsRequestInstance.Customer)
	if err != nil {
		return
	}

	// Fetching from the database
	assets, err = GetAssets(getAssetsRequestInstance.Customer, getAssetsRequestInstance.Location)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
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

func GetValuesHandler(c *gin.Context) {

	var getValuesRequestInstance getValuesRequest

	err := c.BindUri(&getValuesRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Check whether user has access to that customer
	err = helpers.CheckIfUserIsAllowed(c, getValuesRequestInstance.Customer)
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
	processValues, cacheHit := internal.GetDistinctProcessValuesFromCache(
		getValuesRequestInstance.Customer,
		getValuesRequestInstance.Location,
		getValuesRequestInstance.Asset)

	if !cacheHit { // data NOT found
		processValues, err = GetDistinctProcessValues(
			getValuesRequestInstance.Customer,
			getValuesRequestInstance.Location,
			getValuesRequestInstance.Asset)
		if err != nil {
			helpers.HandleInternalServerError(c, err)
			return
		}

		// Store to cache if not yet existing
		go internal.StoreDistinctProcessValuesToCache(
			getValuesRequestInstance.Customer,
			getValuesRequestInstance.Location,
			getValuesRequestInstance.Asset,
			processValues)
		zap.S().Debugf("Stored DistinctProcessValues to cache")
	}

	processValuesString, cacheHit := internal.GetDistinctProcessValuesStringFromCache(
		getValuesRequestInstance.Customer,
		getValuesRequestInstance.Location,
		getValuesRequestInstance.Asset)

	if !cacheHit { // data NOT found
		processValuesString, err = GetDistinctProcessValuesString(
			getValuesRequestInstance.Customer,
			getValuesRequestInstance.Location,
			getValuesRequestInstance.Asset)
		if err != nil {
			helpers.HandleInternalServerError(c, err)
			return
		}

		// Store to cache if not yet existing
		go internal.StoreDistinctProcessValuesStringToCache(
			getValuesRequestInstance.Customer,
			getValuesRequestInstance.Location,
			getValuesRequestInstance.Asset,
			processValuesString)
		zap.S().Debugf("Stored DistinctProcessValuesString to cache")
	}

	values = append(values, processValues...)
	values = append(values, processValuesString...)

	c.JSON(http.StatusOK, values)
}

// ---------------------- getData ----------------------

type getDataRequest struct {
	Customer string `uri:"customer" binding:"required"`
	Location string `uri:"location" binding:"required"`
	Asset    string `uri:"asset" binding:"required"`
	Value    string `uri:"value" binding:"required"`
}

func GetDataHandler(c *gin.Context) {

	var getDataRequestInstance getDataRequest
	var err error

	err = c.BindUri(&getDataRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Check whether user has access to that customer
	err = helpers.CheckIfUserIsAllowed(c, getDataRequestInstance.Customer)
	if err != nil {
		return
	}

	switch getDataRequestInstance.Value {
	case "state":
		processStatesRequest(c, getDataRequestInstance)
	case "count":
		processCountsRequest(c, getDataRequestInstance)
	case "currentState":
		processCurrentStateRequest(c, getDataRequestInstance)
	case "recommendation":
		processRecommendationRequest(c, getDataRequestInstance)
	case "aggregatedStates":
		processAggregatedStatesRequest(c, getDataRequestInstance)
	case "timeRange":
		processTimeRangeRequest(c, getDataRequestInstance)
	case "availability":
		processAvailabilityRequest(c, getDataRequestInstance)
	case "performance":
		processPerformanceRequest(c, getDataRequestInstance)
	case "quality":
		processQualityRequest(c, getDataRequestInstance)
	case "oee":
		processOEERequest(c, getDataRequestInstance)
	case "productionSpeed":
		processProductionSpeedRequest(c, getDataRequestInstance)
	case "qualityRate":
		processQualityRateRequest(c, getDataRequestInstance)
	case "shifts":
		processShiftsRequest(c, getDataRequestInstance)
	case "stateHistogram":
		processStateHistogramRequest(c, getDataRequestInstance)
	case "factoryLocations":
		processFactoryLocationsRequest(c)
	case "averageCleaningTime":
		processAverageCleaningTimeRequest(c, getDataRequestInstance)
	case "averageChangeoverTime":
		processAverageChangeoverTimeRequest(c, getDataRequestInstance)
	case "upcomingMaintenanceActivities":
		processUpcomingMaintenanceActivitiesRequest(c, getDataRequestInstance)
	case "maintenanceComponents":
		processMaintenanceComponentsRequest(c, getDataRequestInstance)
	case "maintenanceActivities":
		processMaintenanceActivitiesRequest(c, getDataRequestInstance)
	case "uniqueProducts":
		processUniqueProductsRequest(c, getDataRequestInstance)
	case "orderTable":
		processOrderTableRequest(c, getDataRequestInstance)
	case "orderTimeline":
		processOrderTimelineRequest(c, getDataRequestInstance)
	case "uniqueProductsWithTags":
		processUniqueProductsWithTagsRequest(c, getDataRequestInstance)
	case "accumulatedProducts":
		processAccumulatedProducts(c, getDataRequestInstance)
	case "unstartedOrderTable":
		processUnstartedOrderTableRequest(c, getDataRequestInstance)
	default:
		if strings.HasPrefix(getDataRequestInstance.Value, "process_") {
			processProcessValueRequest(c, getDataRequestInstance)
		} else if strings.HasPrefix(getDataRequestInstance.Value, "processString_") {
			processProcessValueStringRequest(c, getDataRequestInstance)

		} else {
			helpers.HandleInvalidInputError(c, err)
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

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getStatesRequestInstance getStatesRequest
	var err error

	err = c.BindQuery(&getStatesRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getStatesRequestInstance.From
	to := getStatesRequestInstance.To
	keepStatesInteger := getStatesRequestInstance.KeepStatesInteger

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(customer, location, asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// raw states from database
	rawStates, err := GetStatesRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := processStatesOptimized(
		assetID,
		rawStates,
		rawShifts,
		countSlice,
		orderArray,
		from,
		to,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
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
			unixTimestamp := dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
			t := time.Unix(unixTimestamp, 0)
			formatted := t.Format(time.RFC3339) //formatting the Unix time to RFC3339
			fullRow := []interface{}{
				dataPoint.State,
				formatted}
			data.Datapoints = append(data.Datapoints, fullRow)
		} else {
			unixTimestamp := dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
			t := time.Unix(unixTimestamp, 0)
			formatted := t.Format(time.RFC3339) //formatting the Unix time to RFC3339
			fullRow := []interface{}{
				ConvertStateToString(dataPoint.State, configuration),
				formatted}
			data.Datapoints = append(data.Datapoints, fullRow)
		}
	}

	c.JSON(http.StatusOK, data)
}

// ---------------------- getAggregatedStates ----------------------

type getAggregatedStatesRequest struct {
	From              time.Time `form:"from" binding:"required"`
	To                time.Time `form:"to" binding:"required"`
	IncludeRunning    *bool     `form:"includeRunning" binding:"required"` // *bool is required, see also https://github.com/gin-gonic/gin/issues/814
	KeepStatesInteger bool      `form:"keepStatesInteger"`
	AggregationType   int       `form:"aggregationType"`
}

// processAggregatedStatesRequest gets all states (including running). This can be used to calculate availability.
// If the aggregationType is 0 it will aggregate over the entire time span.
// If the aggregationType is not 0 it will aggregate over various categories, e.g. day or hour
func processAggregatedStatesRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###

	var getAggregatedStatesRequestInstance getAggregatedStatesRequest
	var err error

	err = c.BindQuery(&getAggregatedStatesRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getAggregatedStatesRequestInstance.From
	to := getAggregatedStatesRequestInstance.To
	keepStatesInteger := getAggregatedStatesRequestInstance.KeepStatesInteger
	aggregationType := getAggregatedStatesRequestInstance.AggregationType
	includeRunning := getAggregatedStatesRequestInstance.IncludeRunning

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(customer, location, asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// TODO: parallelize

	// raw states from database
	rawStates, err := GetStatesRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	processedStates, err := processStatesOptimized(
		assetID,
		rawStates,
		rawShifts,
		countSlice,
		orderArray,
		from,
		to,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// TODO: #84 Convert states to string when keepStatesInteger is false and aggregationType is 1

	// Prepare JSON
	var data datamodel.DataResponseAny
	if aggregationType == 0 { // default case. aggregate over everything
		data.ColumnNames = []string{"state", "duration"}

		data.Datapoints, err = CalculateStopParetos(
			processedStates,
			to,
			*includeRunning,
			keepStatesInteger,
			configuration)

		if err != nil {
			helpers.HandleInternalServerError(c, err)
			return
		}
	} else {
		data.ColumnNames = []string{"category", "state", "duration"}

		if aggregationType == 1 { // category: hour in a day

			// create resultDatapoints [][]float64. resultDatapoints[HOUR][STATE] = sum of STATE in that hour
			var resultDatapoints [24][datamodel.MaxState]float64 // 24 hours in a day, 2000 different states (0 - 1999)

			// round up "from" till the next full hour
			tempFrom := time.Date(from.Year(), from.Month(), from.Day(), from.Hour()+1, 0, 0, 0, from.Location())

			if !tempFrom.Before(to) {
				zap.S().Warnf("Not big enough time range (!tempFrom.Before(to))", tempFrom, to)
			}

			// round down "to" till the next full hour
			tempTo := time.Date(to.Year(), to.Month(), to.Day(), to.Hour(), 0, 0, 0, to.Location())

			if !tempTo.After(from) {
				zap.S().Warnf("Not big enough time range (!tempTo.After(from)) %v -> %v", tempTo, from)
			}

			// Call CalculateStopParetos for every hour between "from" and "to" and add results to resultDatapoints
			oldD := tempFrom

			for d := tempFrom; !d.After(tempTo); d = d.Add(time.Hour) { // timestamp is beginning of the state. d is current progress.
				if d == oldD { // if first entry
					continue
				}

				currentHour := d.Hour()

				processedStatesCleaned := removeUnnecessaryElementsFromStateSlice(processedStates, oldD, d)

				var tempResult [][]interface{}
				tempResult, err = CalculateStopParetos(processedStatesCleaned, d, *includeRunning, true, configuration)
				if err != nil {
					helpers.HandleInternalServerError(c, err)
					return
				}

				for _, dataPoint := range tempResult {
					state, ok := dataPoint[0].(int)
					if !ok {
						zap.S().Warnf("Could not convert state to int %v", dataPoint[0])
						continue
					}
					var duration float64
					duration, ok = dataPoint[1].(float64)
					if !ok {
						zap.S().Warnf("Could not convert duration to float64 %v", dataPoint[1])
						continue
					}

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

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getAvailabilityRequestInstance getAvailabilityRequest
	var err error

	err = c.BindQuery(&getAvailabilityRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getAvailabilityRequestInstance.From
	to := getAvailabilityRequestInstance.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(customer, location, asset)
	if err != nil {
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
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

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				to,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAvailability(processedStates, current, to, configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			current = to
		} else { // otherwise, calculate for entire time range

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				currentTo,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAvailability(processedStates, current, currentTo, configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
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

func processPerformanceRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getPerformanceRequestInstance getPerformanceRequest
	var err error

	err = c.BindQuery(&getPerformanceRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getPerformanceRequestInstance.From
	to := getPerformanceRequestInstance.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(customer, location, asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
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

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				to,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculatePerformance(processedStates, current, to, configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			current = to
		} else { // otherwise, calculate for entire time range

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				currentTo,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculatePerformance(processedStates, current, currentTo, configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
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

func processQualityRequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getQualityRequestInstance getQualityRequest
	var err error

	err = c.BindQuery(&getQualityRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getQualityRequestInstance.From
	to := getQualityRequestInstance.To

	// ### fetch necessary data from database ###

	// customer configuration
	_, err = GetCustomerConfiguration(customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
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
			tempDatapoints = CalculateQuality(countSliceSplit)
			current = to
		} else { // otherwise, calculate for entire time range
			// split up countslice that it contains only counts between current and to
			countSliceSplit := SplitCountSlice(countSlice, current, currentTo)

			// calculatequality(c,countslice)
			tempDatapoints = CalculateQuality(countSliceSplit)
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

func processOEERequest(c *gin.Context, getDataRequest getDataRequest) {

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getOEERequestInstance getOEERequest
	var err error

	err = c.BindQuery(&getOEERequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getOEERequestInstance.From
	to := getOEERequestInstance.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(customer, location, asset)
	if err != nil {
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
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

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				to,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateOEE(processedStates, countSliceSplit, current, to, configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			current = to
		} else { // otherwise, calculate for entire time range

			countSliceSplit := SplitCountSlice(countSlice, current, currentTo)

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				currentTo,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateOEE(processedStates, countSliceSplit, current, currentTo, configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
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

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getStateHistogramRequestInstance getStateHistogramRequest
	var err error

	err = c.BindQuery(&getStateHistogramRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getStateHistogramRequestInstance.From
	to := getStateHistogramRequestInstance.To
	includeRunning := getStateHistogramRequestInstance.IncludeRunning
	keepStatesInteger := getStateHistogramRequestInstance.KeepStatesInteger

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(customer, location, asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := processStatesOptimized(
		assetID,
		rawStates,
		rawShifts,
		countSlice,
		orderArray,
		from,
		to,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### create JSON ###
	var data datamodel.DataResponseAny
	data.ColumnNames = []string{"state", "occurrences"}

	data.Datapoints, err = CalculateStateHistogram(processedStates, includeRunning, keepStatesInteger, configuration)

	if err != nil {
		helpers.HandleInternalServerError(c, err)
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

type getProcessValueStringRequest struct {
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

func processCurrentStateRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getCurrentStateRequestInstance getCurrentStateRequest
	var err error

	err = c.BindQuery(&getCurrentStateRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	// TODO: #89 Return timestamps in RFC3339 in /currentState
	state, err := GetCurrentState(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getCurrentStateRequestInstance.KeepStatesInteger)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func processCountsRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getCountsRequestInstance getCountsRequest
	var err error
	var counts datamodel.DataResponseAny

	err = c.BindQuery(&getCountsRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	// TODO: #88 Return timestamps in RFC3339 in /counts
	counts, err = GetCounts(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getCountsRequestInstance.From,
		getCountsRequestInstance.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

func processRecommendationRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Fetching from the database
	recommendations, err := GetRecommendations(getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, recommendations)
}

func processShiftsRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getShiftsRequestInstance getShiftsRequest
	var err error

	err = c.BindQuery(&getShiftsRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	shifts, err := GetShifts(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getShiftsRequestInstance.From,
		getShiftsRequestInstance.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, shifts)
}

func processProcessValueRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getProcessValueRequestInstance getProcessValueRequest
	var err error

	err = c.BindQuery(&getProcessValueRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	valueName := strings.TrimPrefix(getDataRequest.Value, "process_")

	// TODO: #96 Return timestamps in RFC3339 in /processValue

	// Fetching from the database
	processValues, err := GetProcessValue(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getProcessValueRequestInstance.From,
		getProcessValueRequestInstance.To,
		valueName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, processValues)
}

func processProcessValueStringRequest(c *gin.Context, getDataRequest getDataRequest) {
	var getProcessValueStringRequestInstance getProcessValueStringRequest
	var err error

	err = c.BindQuery(&getProcessValueStringRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	valueName := strings.TrimPrefix(getDataRequest.Value, "processString_")

	zap.S().Debugf("%s", valueName)
	// TODO: #96 Return timestamps in RFC3339 in /processValueString

	// Fetching from the database
	processValuesString, err := GetProcessValueString(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getProcessValueStringRequestInstance.From,
		getProcessValueStringRequestInstance.To,
		valueName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, processValuesString)
}

func processTimeRangeRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Fetching from the database
	timeRange, err := GetDataTimeRangeForAsset(getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, timeRange)
}

func processUpcomingMaintenanceActivitiesRequest(c *gin.Context, getDataRequest getDataRequest) {

	rawData, err := GetUpcomingTimeBasedMaintenanceActivities(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	var data datamodel.DataResponseAny
	data.ColumnNames = []string{"Machine", "Component", "Activity", "Duration", "Status"}

	// customer configuration
	configuration, err := GetCustomerConfiguration(getDataRequest.Customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// TODO: #100 Return timestamps in RFC3339 in /maintenanceActivities

	for _, timeBasedMaintenanceActivity := range rawData {
		var activityString = ConvertActivityToString(timeBasedMaintenanceActivity.ActivityType, configuration)

		if !timeBasedMaintenanceActivity.DurationInDays.Valid || !timeBasedMaintenanceActivity.LatestActivity.Valid || !timeBasedMaintenanceActivity.NextActivity.Valid {
			fullRow := []interface{}{
				getDataRequest.Asset,
				timeBasedMaintenanceActivity.ComponentName,
				activityString,
				0,
				0}
			data.Datapoints = append(data.Datapoints, fullRow)
		} else {
			var status = 2                                                // green by default
			if timeBasedMaintenanceActivity.DurationInDays.Float64 <= 0 { // critical
				status = 0
			} else if timeBasedMaintenanceActivity.DurationInDays.Float64*24/float64(timeBasedMaintenanceActivity.IntervallInHours) < 0.3 { // under a third of the runtime we are on oragne area
				status = 1
			}

			fullRow := []interface{}{
				getDataRequest.Asset,
				timeBasedMaintenanceActivity.ComponentName,
				activityString,
				timeBasedMaintenanceActivity.DurationInDays.Float64,
				status}
			data.Datapoints = append(data.Datapoints, fullRow)
		}
	}

	c.JSON(http.StatusOK, data)
}

func processUnstartedOrderTableRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getOrderRequestInstance getOrderRequest
	var err error

	err = c.BindQuery(&getOrderRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetch data from database
	zap.S().Debugf(
		"Fetching order table for customer %s, location %s, asset %s, value: %v",
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getDataRequest.Value)

	zap.S().Debugf("GetUnstartedOrdersRaw")
	rawOrders, err := GetUnstartedOrdersRaw(getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	data := datamodel.DataResponseAny{}
	data.ColumnNames = []string{"OrderName", "ProductName", "TargetUnits", "TimePerUnitInSeconds"}
	for _, order := range rawOrders {
		data.Datapoints = append(
			data.Datapoints,
			[]interface{}{order.OrderName, order.ProductName, order.TargetUnits, order.TimePerUnitInSeconds})
	}

	c.JSON(http.StatusOK, data)
}

func processOrderTableRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getOrderRequestInstance getOrderRequest
	var err error

	err = c.BindQuery(&getOrderRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetch data from database
	zap.S().Debugf(
		"Fetching order table for customer %s, location %s, asset %s, value: %v",
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getDataRequest.Value)

	// customer configuration
	zap.S().Debugf("GetCustomerConfiguration")
	configuration, err := GetCustomerConfiguration(getDataRequest.Customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	zap.S().Debugf("GetAssetID")
	assetID, err := GetAssetID(getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	zap.S().Debugf("GetOrdersRaw")
	rawOrders, err := GetOrdersRaw(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getOrderRequestInstance.From,
		getOrderRequestInstance.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for actual units calculation
	zap.S().Debugf("GetCountsRaw")
	countSlice, err := GetCountsRaw(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getOrderRequestInstance.From,
		getOrderRequestInstance.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// raw states from database
	zap.S().Debugf("GetStatesRaw")
	rawStates, err := GetStatesRaw(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getOrderRequestInstance.From,
		getOrderRequestInstance.To,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	zap.S().Debugf("GetShiftsRaw")
	rawShifts, err := GetShiftsRaw(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getOrderRequestInstance.From,
		getOrderRequestInstance.To,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// TODO: #98 Return timestamps in RFC3339 in /orderTable

	// Process data
	// zap.S().Debugf("calculateOrderInformation: rawOrders: %v, countSlice: %v, assetID: %v, rawStates: %v, rawShifts: %v, configuration: %v, Location: %v, Asset: %v", rawOrders, countSlice, assetID, rawStates, rawShifts, configuration, getDataRequest.Location, getDataRequest.Asset)
	data, err := calculateOrderInformation(
		rawOrders,
		countSlice,
		assetID,
		rawStates,
		rawShifts,
		configuration,
		getDataRequest.Location,
		getDataRequest.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func processOrderTimelineRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getOrderRequestInstance getOrderRequest
	var err error

	err = c.BindQuery(&getOrderRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// TODO: #97 Return timestamps in RFC3339 in /orderTimeline

	// Process data
	data, err := GetOrdersTimeline(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getOrderRequestInstance.From,
		getOrderRequestInstance.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func processMaintenanceActivitiesRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Fetching from the database
	data, err := GetMaintenanceActivities(getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func processUniqueProductsRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getUniqueProductsRequestInstance getUniqueProductsRequest
	var err error

	err = c.BindQuery(&getUniqueProductsRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// TODO: #99 Return timestamps in RFC3339 in /uniqueProducts

	// Fetching from the database
	uniqueProducts, err := GetUniqueProducts(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getUniqueProductsRequestInstance.From,
		getUniqueProductsRequestInstance.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, uniqueProducts)
}

func processMaintenanceComponentsRequest(c *gin.Context, getDataRequest getDataRequest) {

	// Fetching from the database
	assetID, err := GetAssetID(getDataRequest.Customer, getDataRequest.Location, getDataRequest.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	data, err := GetComponents(assetID)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func processProductionSpeedRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getProductionSpeedRequestInstance getProductionSpeedRequest
	var err error
	var counts datamodel.DataResponseAny

	err = c.BindQuery(&getProductionSpeedRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	counts, err = GetProductionSpeed(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getProductionSpeedRequestInstance.From,
		getProductionSpeedRequestInstance.To,
		getProductionSpeedRequestInstance.AggregationInterval)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

func processQualityRateRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getQualityRateRequestInstance getQualityRateRequest
	var err error
	var counts datamodel.DataResponseAny

	err = c.BindQuery(&getQualityRateRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	counts, err = GetQualityRate(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getQualityRateRequestInstance.From,
		getQualityRateRequestInstance.To,
		getQualityRateRequestInstance.AggregationInterval)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

func processFactoryLocationsRequest(c *gin.Context) {

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

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getAverageCleaningTimeRequestInstance getAverageCleaningTimeRequest
	var err error

	err = c.BindQuery(&getAverageCleaningTimeRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getAverageCleaningTimeRequestInstance.From
	to := getAverageCleaningTimeRequestInstance.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(customer, location, asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
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

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				to,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(
				processedStates,
				current,
				to,
				configuration,
				18) // Cleaning is 18
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			current = to
		} else { // otherwise, calculate for entire time range

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				currentTo,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(processedStates, current, currentTo, configuration, 18)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
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

	// ### store getDataRequest in proper variables ###
	customer := getDataRequest.Customer
	location := getDataRequest.Location
	asset := getDataRequest.Asset

	// ### parse query ###
	var getAverageChangeoverTimeRequestInstance getAverageChangeoverTimeRequest
	var err error

	err = c.BindQuery(&getAverageChangeoverTimeRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getAverageChangeoverTimeRequestInstance.From
	to := getAverageChangeoverTimeRequestInstance.To

	// ### fetch necessary data from database ###

	assetID, err := GetAssetID(customer, location, asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetCustomerConfiguration(customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(customer, location, asset, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(customer, location, asset, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
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

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				to,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(
				processedStates,
				current,
				to,
				configuration,
				datamodel.ChangeoverState)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			current = to
		} else { // otherwise, calculate for entire time range

			processedStates, err := processStates(
				assetID,
				rawStates,
				rawShifts,
				countSlice,
				orderArray,
				current,
				currentTo,
				configuration)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}

			tempDatapoints, err = CalculateAverageStateTime(
				processedStates,
				current,
				currentTo,
				configuration,
				datamodel.ChangeoverState)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
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

func processUniqueProductsWithTagsRequest(c *gin.Context, getDataRequest getDataRequest) {

	var getUniqueProductsWithTagsRequestInstance getUniqueProductsWithTagsRequest
	var err error

	err = c.BindQuery(&getUniqueProductsWithTagsRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	uniqueProductsWithTags, err := GetUniqueProductsWithTags(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getUniqueProductsWithTagsRequestInstance.From,
		getUniqueProductsWithTagsRequestInstance.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, uniqueProductsWithTags)
}

type getProcessAccumulatedProducts struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

func processAccumulatedProducts(c *gin.Context, getDataRequest getDataRequest) {

	var getProcessAccumulatedProductsInstance getProcessAccumulatedProducts
	var err error

	err = c.BindQuery(&getProcessAccumulatedProductsInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	accumulatedProducts, err := GetAccumulatedProducts(
		getDataRequest.Customer,
		getDataRequest.Location,
		getDataRequest.Asset,
		getProcessAccumulatedProductsInstance.From,
		getProcessAccumulatedProductsInstance.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, accumulatedProducts)

}
