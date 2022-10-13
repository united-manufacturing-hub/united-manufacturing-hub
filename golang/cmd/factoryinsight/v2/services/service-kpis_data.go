package services

import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"net/http"
)

func ProcessOeeKpiRequest(c *gin.Context, request models.GetKpisDataRequest) {
	// TODO adapt this to the new data model
	// ### store request values in proper variables ###
	enterprise := request.EnterpriseName
	site := request.SiteName
	area := request.AreaName
	productLine := request.ProductionLineName
	workCell := request.WorkCellName

	// ### parse query ###
	var oeeKpiRequest models.KpiRequest
	var err error

	err = c.BindUri(&oeeKpiRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := oeeKpiRequest.From
	to := oeeKpiRequest.To

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

func ProcessAvailabilityKpiRequest(c *gin.Context, request models.GetKpisDataRequest) {
	// TODO adapt this to the new data model
	// ### store request in proper variables ###
	enterprise := request.EnterpriseName
	site := request.SiteName
	area := request.AreaName
	productLine := request.ProductionLineName
	workCell := request.WorkCellName

	// ### parse query ###
	var availabilityKpiRequest models.KpiRequest
	var err error

	err = c.BindQuery(&availabilityKpiRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := availabilityKpiRequest.From
	to := availabilityKpiRequest.To

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

func ProcessPerformanceKpiRequest(c *gin.Context, request models.GetKpisDataRequest) {
	// TODO adapt this to the new data model
	// ### store request in proper variables ###
	enterprise := request.EnterpriseName
	site := request.SiteName
	area := request.AreaName
	productLine := request.ProductionLineName
	workCell := request.WorkCellName

	// ### parse query ###
	var performanceKpiRequest models.KpiRequest
	var err error

	err = c.BindQuery(&performanceKpiRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := performanceKpiRequest.From
	to := performanceKpiRequest.To

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

func ProcessQualityKpiRequest(c *gin.Context, request models.GetKpisDataRequest) {
	// TODO adapt this to the new data model
	// ### store request in proper variables ###
	enterprise := request.EnterpriseName
	site := request.SiteName
	area := request.AreaName
	productLine := request.ProductionLineName
	workCell := request.WorkCellName

	// ### parse query ###
	var qualityKpiRequest models.KpiRequest
	var err error

	err = c.BindQuery(&qualityKpiRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := qualityKpiRequest.From
	to := qualityKpiRequest.To

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
