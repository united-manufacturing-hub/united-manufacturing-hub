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

package services

import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/repository"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"net/http"
)

func GetKpisMethods(enterpriseName, siteName, workCellName string) (kpis models.GetKpisMethodsResponse, err error) {

	zap.S().Debugf("[GetKpisMethods] Getting KPIs for enterprise %s, site %s and work cell %s", enterpriseName, siteName, workCellName)

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		zap.S().Debugf("[GetKpisMethods] Work cell %s not found", workCellName)
		return
	}
	var stateExists bool
	stateExists, err = GetStateExists(workCellId)
	if err != nil {
		zap.S().Debugf("[GetKpisMethods] Error while checking if state exists")
		return
	}
	// All KPI's require state

	if stateExists {
		zap.S().Debugf("[GetKpisMethods] State exists")
		kpis.Kpis = append(kpis.Kpis, models.OeeKpi, models.AvailabilityKpi, models.PerformanceKpi, models.QualityKpi)
	}

	return
}

func ProcessOeeKpiRequest(c *gin.Context, request models.GetKpisDataRequest) {
	// ### store request values in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	// ### parse query ###
	var getOeeKpiRequest models.GetOeeKpiRequest

	err := c.BindQuery(&getOeeKpiRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getOeeKpiRequest.From
	to := getOeeKpiRequest.To

	// ### fetch necessary data from database ###

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetEnterpriseConfiguration(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := enterpriseName + "-" + siteName + "-" + areaName + "-" + productionLineName + "-" + workCellName + "-" + "oee"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: #85 Ensure that multi-day OEE is split up during multiples 00:00 instead of multiples of the from time.

	// TODO: create JSON and calculate in the same paragraph
	for current := from; current != to; {
		var tempDatapoints []interface{}

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value

			countSliceSplit := repository.SplitCountSlice(countSlice, current, to)

			processedStates, err := ProcessStates(
				workCellId,
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

			countSliceSplit := repository.SplitCountSlice(countSlice, current, currentTo)

			processedStates, err := ProcessStates(
				workCellId,
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
	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	// ### parse query ###
	var getAvailabilityKpiRequest models.GetAvailabilityKpiRequest
	var err error

	err = c.BindQuery(&getAvailabilityKpiRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getAvailabilityKpiRequest.From
	to := getAvailabilityKpiRequest.To

	// ### fetch necessary data from database ###

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetEnterpriseConfiguration(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := enterpriseName + "-" + siteName + "-" + areaName + "-" + productionLineName + "-" + workCellName + "-" + "oee"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: #85 Ensure that multi-day OEE is split up during multiples 00:00 instead of multiples of the from time.

	for current := from; current != to; {
		var tempDatapoints []interface{}

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value

			processedStates, err := ProcessStates(
				workCellId,
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

			processedStates, err := ProcessStates(
				workCellId,
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
	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	// ### parse query ###
	var getPerformanceKpiRequest models.GetPerformanceKpiRequest
	var err error

	err = c.BindQuery(&getPerformanceKpiRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getPerformanceKpiRequest.From
	to := getPerformanceKpiRequest.To

	// ### fetch necessary data from database ###

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetEnterpriseConfiguration(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// raw states from database
	rawStates, err := GetStatesRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := enterpriseName + "-" + siteName + "-" + areaName + "-" + productionLineName + "-" + workCellName + "-" + "oee"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: #85 Ensure that multi-day OEE is split up during multiples 00:00 instead of multiples of the from time.

	for current := from; current != to; {
		var tempDatapoints []interface{}

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value

			processedStates, err := ProcessStates(
				workCellId,
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

			processedStates, err := ProcessStates(
				workCellId,
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
	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	// ### parse query ###
	var getQualityKpiRequest models.GetQualityKpiRequest
	var err error

	err = c.BindQuery(&getQualityKpiRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getQualityKpiRequest.From
	to := getQualityKpiRequest.To

	// ### fetch necessary data from database ###

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	_, err = GetEnterpriseConfiguration(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###

	// ### create JSON ###
	var data datamodel.DataResponseAny
	JSONColumnName := enterpriseName + "-" + siteName + "-" + areaName + "-" + productionLineName + "-" + workCellName + "-" + "quality"
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
