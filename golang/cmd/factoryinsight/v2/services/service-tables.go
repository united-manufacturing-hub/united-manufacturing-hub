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
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"net/http"
	"time"
)

func GetTableTypes(enterpriseName string, siteName string, workCellName string) (
	tables models.GetTableTypesResponse,
	err error) {

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		return
	}
	var stateExists bool
	stateExists, err = GetStateExists(workCellId)
	if err != nil {
		return
	}

	var countExists bool
	countExists, err = GetOutputExists(workCellId)
	if err != nil {
		return
	}

	var orderExists bool
	orderExists, err = GetJobsExists(workCellId)
	if err != nil {
		return
	}
	/*
		var shiftExists bool
		shiftExists, err = GetShiftExists(workCellId)
		if err != nil {
			return
		}
	*/
	var productExists bool
	productExists, err = GetProductExists(workCellId)
	if err != nil {
		return
	}

	var uniqueProductsExists bool
	uniqueProductsExists, err = GetUniqueProductsExists(workCellId)
	if err != nil {
		return
	}

	if orderExists {
		tables.Tables = append(tables.Tables, models.TableType{Id: 0, Name: models.JobsTable})
	}
	if uniqueProductsExists {
		tables.Tables = append(tables.Tables, models.TableType{Id: 1, Name: models.ProductsTable})
	}
	if orderExists && countExists && productExists {
		tables.Tables = append(tables.Tables, models.TableType{Id: 2, Name: models.ProductTypesTable})
	}

	if stateExists {
		tables.Tables = append(tables.Tables, models.TableType{Id: 3, Name: models.AvailabilityHistogramTable})
		tables.Tables = append(tables.Tables, models.TableType{Id: 4, Name: models.AvailabilityTotalTable})
	}
	/*
		// NOT YET IMPLEMENTED !
		tables.Tables = append(tables.Tables, models.TableType{Id: 5, Name: models.PerformanceTable})
		tables.Tables = append(tables.Tables, models.TableType{Id: 6, Name: models.QualityTable})
	*/
	return
}

func ProcessJobsTableRequest(c *gin.Context, request models.GetTableDataRequest) {
	// TODO adapt this to the new data model
	// ### store request values in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	// ### parse query ###
	var getJobTableRequest models.GetJobTableRequest
	var err error

	err = c.BindQuery(&getJobTableRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetch data from database
	zap.S().Debugf(
		"Fetching order table for enterprise %s, site %s, area %s, production line: %s, work cell: %s",
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName)

	// customer configuration
	zap.S().Debugf("GetEnterpriseConfiguration")
	configuration, err := GetEnterpriseConfiguration(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	zap.S().Debugf("GetWorkCellId")
	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	zap.S().Debugf("GetOrdersRaw")
	rawOrders, err := GetOrdersRaw(
		workCellId,
		getJobTableRequest.From,
		getJobTableRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for actual units calculation
	zap.S().Debugf("GetCountsRaw")
	countSlice, err := GetCountsRaw(
		workCellId,
		getJobTableRequest.From,
		getJobTableRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// raw states from database
	zap.S().Debugf("GetStatesRaw")
	rawStates, err := GetStatesRaw(
		workCellId,
		getJobTableRequest.From,
		getJobTableRequest.To,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	zap.S().Debugf("GetShiftsRaw")
	rawShifts, err := GetShiftsRaw(
		workCellId,
		getJobTableRequest.From,
		getJobTableRequest.To,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// TODO: #98 Return timestamps in RFC3339 in /orderTable

	// Process data
	// zap.S().Debugf("calculateOrderInformation: rawOrders: %v, countSlice: %v, workCellId: %v, rawStates: %v, rawShifts: %v, configuration: %v, Location: %v, Asset: %v", rawOrders, countSlice, workCellId, rawStates, rawShifts, configuration, request.Location, request.Asset)
	data, err := CalculateOrderInformation(
		rawOrders,
		countSlice,
		workCellId,
		rawStates,
		rawShifts,
		configuration,
		siteName,
		workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func ProcessProductsTableRequest(c *gin.Context, request models.GetTableDataRequest) {
	// TODO adapt this to the new data model
	// ### store request values in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	workCellName := request.WorkCellName

	// ### parse query ###
	var getProductsTableRequest models.GetProductsTableRequest
	var err error

	err = c.BindQuery(&getProductsTableRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	zap.S().Debugf("GetWorkCellId")
	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// TODO: #99 Return timestamps in RFC3339 in /products

	// Fetching from the database
	products, err := getUniqueProducts(
		workCellId,
		getProductsTableRequest.From,
		getProductsTableRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, products)
}

func ProcessProductTypesTableRequest(c *gin.Context, request models.GetTableDataRequest) {
	// TODO adapt this to the new data model
	// ### store request values in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	workCellName := request.WorkCellName

	// ### parse query ###
	var getProductTypesTableRequest models.GetProductTypesTableRequest
	var err error

	err = c.BindQuery(&getProductTypesTableRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database

	zap.S().Debugf("GetWorkCellId")
	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	productTypes, err := getProductTypes(
		workCellId,
		getProductTypesTableRequest.From,
		getProductTypesTableRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, productTypes)
}

func ProcessAvailabilityHistogramTableRequest(c *gin.Context, request models.GetTableDataRequest) {

	// ### store request values in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	workCellName := request.WorkCellName

	// ### parse query ###
	var getShopfloorLossesTableRequest models.GetAvailabilityHistogramRequest
	var err error

	err = c.BindQuery(&getShopfloorLossesTableRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getShopfloorLossesTableRequest.From
	to := getShopfloorLossesTableRequest.To
	includeRunning := helpers.StrToBool(getShopfloorLossesTableRequest.IncludeRunning)
	keepStatesInteger := helpers.StrToBool(getShopfloorLossesTableRequest.KeepStatesInteger)

	var workCellId uint32
	workCellId, err = GetWorkCellId(enterpriseName, siteName, workCellName)
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
	processedStates, err := ProcessStatesOptimized(
		workCellId,
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

func ProcessAvailabilityTotalTableRequest(c *gin.Context, request models.GetTableDataRequest) {

	// ### store getDataRequest in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	workCellName := request.WorkCellName

	// ### parse query ###

	var getAggregatedStatesRequestInstance models.GetAggregatedStatesRequest
	var err error

	err = c.BindQuery(&getAggregatedStatesRequestInstance)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getAggregatedStatesRequestInstance.From
	to := getAggregatedStatesRequestInstance.To
	keepStatesInteger := helpers.StrToBool(getAggregatedStatesRequestInstance.KeepStatesInteger)
	aggregationType := getAggregatedStatesRequestInstance.AggregationType
	includeRunning := helpers.StrToBool(getAggregatedStatesRequestInstance.IncludeRunning)

	// ### fetch necessary data from database ###

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// enterpriseName configuration
	configuration, err := GetEnterpriseConfiguration(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	// TODO: parallelize

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

	processedStates, err := ProcessStatesOptimized(
		workCellId,
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
			includeRunning,
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

				processedStatesCleaned := RemoveUnnecessaryElementsFromStateSlice(processedStates, oldD, d)

				var tempResult [][]interface{}
				tempResult, err = CalculateStopParetos(processedStatesCleaned, d, includeRunning, true, configuration)
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

func ProcessPerformanceTableRequest(c *gin.Context, request models.GetTableDataRequest) {
	c.String(http.StatusNotImplemented, "Not implemented yet")
}

func ProcessQualityTableRequest(c *gin.Context, request models.GetTableDataRequest) {
	c.String(http.StatusNotImplemented, "Not implemented yet")
}

// GetUniqueProducts gets all unique products for a specific asset in a specific time range
func getUniqueProducts(workCellId uint32, from, to time.Time) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetUniqueProducts] work cell: %v from: %v, to: %v",
		workCellId,
		from,
		to)

	data.ColumnNames = []string{"UID", "AID", "TimestampBegin", "TimestampEnd", "ProductID", "IsScrap"}

	sqlStatement := `
	SELECT uniqueProductID, uniqueProductAlternativeID, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap 
	FROM uniqueProductTable 
	WHERE asset_id = $1 
		AND (begin_timestamp_ms BETWEEN $2 AND $3 OR end_timestamp_ms BETWEEN $2 AND $3) 
		OR (begin_timestamp_ms < $2 AND end_timestamp_ms > $3) 
	ORDER BY begin_timestamp_ms ASC;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, workCellId, from, to)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	defer rows.Close()

	for rows.Next() {

		var UID int
		var AID string
		var timestampBegin time.Time
		var timestampEnd sql.NullTime
		var productID int
		var isScrap bool

		err = rows.Scan(&UID, &AID, &timestampBegin, &timestampEnd, &productID, &isScrap)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}
		var fullRow []interface{}
		fullRow = append(fullRow, UID)
		fullRow = append(fullRow, AID)
		fullRow = append(fullRow, float64(timestampBegin.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
		if timestampEnd.Valid {
			fullRow = append(
				fullRow,
				float64(timestampEnd.Time.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
		} else {
			fullRow = append(fullRow, nil)
		}
		fullRow = append(fullRow, productID)
		fullRow = append(fullRow, isScrap)

		data.Datapoints = append(data.Datapoints, fullRow)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	// CheckOutputDimensions checks, if the length of columnNames corresponds to the length of each row of data
	err = CheckOutputDimensions(data.Datapoints, data.ColumnNames)
	if err != nil {

		return
	}
	return
}

// getProductTypes gets the accumulated counts for an observation timeframe and an asset
// old GetAccumulatedProducts
func getProductTypes(workCellId uint32, from, to time.Time) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetUniqueProductsWithTags] work cell: %v from: %v, to: %v",
		workCellId,
		from,
		to)

	zap.S().Debugf("Request ts: %d -> %d", from.UnixMilli(), to.UnixMilli())

	// Selects orders outside observation range
	sqlStatementGetOutsider := `
SELECT ot.order_id, ot.product_id, ot.begin_timestamp, ot.end_timestamp, ot.target_units, ot.asset_id FROM ordertable ot
WHERE
      ot.asset_id = $1
  AND
      ot.begin_timestamp IS NOT NULL
AND (
                ot.begin_timestamp <= $2
            AND
                ot.end_timestamp IS NULL
        OR
                ot.end_timestamp >= $2
    )
ORDER BY begin_timestamp ASC
LIMIT 1;
`
	// Select orders inside observation range
	sqlStatementGetInsiders := `
SELECT ot.order_id, ot.product_id, ot.begin_timestamp, ot.end_timestamp, ot.target_units, ot.asset_id FROM ordertable ot
WHERE ot.asset_id = $1
AND (
          ot.begin_timestamp >= $2
          AND
          ot.begin_timestamp <= $3
          )
AND ot.order_id != $4
ORDER BY begin_timestamp ASC
;
`
	// Select orders inside observation range, if there are no outsiders
	sqlStatementGetInsidersNoOutsider := `
SELECT ot.order_id, ot.product_id, ot.begin_timestamp, ot.end_timestamp, ot.target_units, ot.asset_id FROM ordertable ot
WHERE ot.asset_id = $1
AND (
          ot.begin_timestamp >= $2
          AND
          ot.begin_timestamp <= $3
          )
ORDER BY begin_timestamp ASC
;
`

	// Get order outside observation window
	row := database.Db.QueryRow(sqlStatementGetOutsider, workCellId, from)
	err = row.Err()
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Debugf("No outsider rows")
		// We don't care if there is no outside order, in this case we will just select all insider orders
	} else if err != nil {
		database.ErrorHandling(sqlStatementGetOutsider, err, false)

		return
	}

	// Holds an order, retrieved from our DB
	type Order struct {
		timestampBegin time.Time
		timestampEnd   sql.NullTime
		OID            int
		PID            int
		AID            int
		targetUnits    sql.NullInt32
	}

	// Order that has started before observation time
	var OuterOrder Order

	var OidOuter int
	var PidOuter int
	var timestampbeginOuter time.Time
	var timestampendOuter sql.NullTime
	var targetunitsOuter sql.NullInt32
	var AidOuter int
	foundOutsider := true

	err = row.Scan(&OidOuter, &PidOuter, &timestampbeginOuter, &timestampendOuter, &targetunitsOuter, &AidOuter)

	OuterOrder = Order{
		OID:            OidOuter,
		PID:            PidOuter,
		timestampBegin: timestampbeginOuter,
		timestampEnd:   timestampendOuter,
		targetUnits:    targetunitsOuter,
		AID:            AidOuter,
	}

	if errors.Is(err, sql.ErrNoRows) {
		foundOutsider = false
	} else if err != nil {
		database.ErrorHandling(sqlStatementGetOutsider, err, false)

		return
	}

	var insideOrderRows *sql.Rows
	if foundOutsider {
		// Get insiders without the outsider order
		zap.S().Debugf("Query with outsider: ", OuterOrder)
		insideOrderRows, err = database.Db.Query(sqlStatementGetInsiders, workCellId, from, to, OuterOrder.OID)
	} else {
		// Get insiders
		zap.S().Debugf("Query without outsider: ", OuterOrder)
		insideOrderRows, err = database.Db.Query(sqlStatementGetInsidersNoOutsider, workCellId, from, to)
	}

	if errors.Is(err, sql.ErrNoRows) {
		// It is valid to have no internal rows !
		zap.S().Debugf("No internal rows")
	} else if err != nil {
		database.ErrorHandling(sqlStatementGetInsidersNoOutsider, err, false)

		return
	}

	// List of all inside orders
	var insideOrders []Order

	foundInsider := false
	for insideOrderRows.Next() {

		var OID int
		var PID int
		var timestampBegin time.Time
		var timestampEnd sql.NullTime
		var targetUnits sql.NullInt32
		var AID int
		err = insideOrderRows.Scan(&OID, &PID, &timestampBegin, &timestampEnd, &targetUnits, &AID)
		if err != nil {
			database.ErrorHandling(sqlStatementGetInsidersNoOutsider, err, false)

			return
		}
		foundInsider = true
		zap.S().Debugf(
			"Found insider: %d, %d, %s, %s, %d, %d",
			OID,
			PID,
			timestampBegin,
			timestampEnd,
			targetUnits,
			AID)
		insideOrders = append(
			insideOrders, Order{
				OID:            OID,
				PID:            PID,
				timestampBegin: timestampBegin,
				timestampEnd:   timestampEnd,
				targetUnits:    targetUnits,
				AID:            AID,
			})
	}

	var observationStart time.Time
	var observationEnd time.Time

	if !foundInsider && !foundOutsider {
		zap.S().Debugf("No insiders or outsiders !")
		observationStart = from
		observationEnd = to
	} else {

		// If value before observation window, use it's begin timestamp
		// Else iter all inside rows and select the lowest timestamp
		if foundOutsider {
			observationStart = OuterOrder.timestampBegin
		} else {
			observationStart = time.Unix(1<<16-1, 0)
			for _, rowdatum := range insideOrders {
				if rowdatum.timestampBegin.Before(observationStart) {
					observationStart = rowdatum.timestampBegin
				}
			}
		}

		observationEnd = time.Unix(0, 0)
		// If value inside observation window, iterate them and select the greatest time.
		// If order has no end, assume unix max time
		if foundInsider {
			for _, rowdatum := range insideOrders {
				if rowdatum.timestampEnd.Valid {
					if rowdatum.timestampEnd.Time.After(observationEnd) {
						observationEnd = rowdatum.timestampEnd.Time
						zap.S().Debugf("[1992] Set observationEnd %s", observationEnd.String())
					}
				} else {
					if time.Unix(1<<16-1, 0).After(observationEnd) {
						observationEnd = time.Unix(1<<16-1, 0)
						zap.S().Debugf("[1996] Set observationEnd %s", observationEnd.String())
					}
				}
			}
		}
		// Check if our starting order has the largest end time
		// Also assign unix max time, if there is still no valid value
		if OuterOrder.timestampEnd.Valid {
			if OuterOrder.timestampEnd.Time.After(observationEnd) {
				observationEnd = OuterOrder.timestampEnd.Time
				zap.S().Debugf("[2005] Set observationEnd %s", observationEnd.String())
			}
		} else if observationEnd.Equal(time.Unix(0, 0)) {
			observationEnd = to
			zap.S().Debugf("[2009] Set observationEnd %s", observationEnd.String())
		}
	}

	if observationStart.After(observationEnd) {
		zap.S().Warnf("observationStart > observationEnd: %s > %s", observationStart.String(), observationEnd.String())
	}

	zap.S().Debugf("Set observation start to: %s", observationStart)
	zap.S().Debugf("Set observation end to: %s", observationEnd)

	// Get all counts
	var sqlStatementGetCounts = `SELECT timestamp, count, scrap FROM counttable WHERE asset_id = $1 AND timestamp >= to_timestamp($2::double precision) AND timestamp <= to_timestamp($3::double precision) ORDER BY timestamp ASC;`

	countQueryBegin := observationStart.UnixMilli()
	countQueryEnd := int64(0)
	if to.After(observationEnd) {
		countQueryEnd = to.UnixMilli()
	} else {
		countQueryEnd = observationEnd.UnixMilli()
	}

	var countRows *sql.Rows
	countRows, err = database.Db.Query(
		sqlStatementGetCounts,
		workCellId,
		float64(countQueryBegin)/1000,
		float64(countQueryEnd)/1000)

	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatementGetCounts, err, false)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatementGetCounts, err, false)

		return
	}
	defer countRows.Close()

	countMap := make([]models.CountStruct, 0)

	for countRows.Next() {
		var timestamp time.Time
		var count int32
		var scrapN sql.NullInt32
		err = countRows.Scan(&timestamp, &count, &scrapN)

		if err != nil {
			database.ErrorHandling(sqlStatementGetCounts, err, false)

			return
		}

		var scrap int32
		if scrapN.Valid {
			scrap = scrapN.Int32
		}

		countMap = append(countMap, models.CountStruct{Timestamp: timestamp, Count: int(count), Scrap: int(scrap)})
	}

	// Get all orders in timeframe
	sqlGetRunningOrders := `SELECT order_id, product_id, target_units, begin_timestamp, end_timestamp FROM ordertable WHERE asset_id = $1 AND begin_timestamp < to_timestamp($2::double precision) AND end_timestamp >= to_timestamp($3::double precision) OR end_timestamp = NULL`

	orderQueryBegin := observationStart.UnixMilli()
	orderQueryEnd := int64(0)
	if to.After(observationEnd) {
		orderQueryEnd = to.UnixMilli()
	} else {
		orderQueryEnd = observationEnd.UnixMilli()
	}

	var orderRows *sql.Rows
	orderRows, err = database.Db.Query(
		sqlGetRunningOrders,
		workCellId,
		float64(orderQueryEnd)/1000,
		float64(orderQueryBegin)/1000)

	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlGetRunningOrders, err, false)
		return
	} else if err != nil {
		database.ErrorHandling(sqlGetRunningOrders, err, false)

		return
	}
	defer orderRows.Close()

	orderMap := make([]models.OrderStruct, 0)

	for orderRows.Next() {
		var orderID int
		var productId int
		var targetUnits int
		var beginTimeStamp time.Time
		var endTimeStamp sql.NullTime
		err = orderRows.Scan(&orderID, &productId, &targetUnits, &beginTimeStamp, &endTimeStamp)

		if err != nil {
			database.ErrorHandling(sqlGetRunningOrders, err, false)

			return
		}

		orderMap = append(
			orderMap, models.OrderStruct{
				OrderID:        orderID,
				ProductId:      productId,
				TargetUnits:    targetUnits,
				BeginTimeStamp: beginTimeStamp,
				EndTimeStamp:   endTimeStamp,
			})
	}

	sqlGetProductsPerSec := `SELECT product_id, time_per_unit_in_seconds FROM producttable WHERE asset_id = $1`

	var productRows *sql.Rows
	productRows, err = database.Db.Query(sqlGetProductsPerSec, workCellId)

	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlGetProductsPerSec, err, false)
		return
	} else if err != nil {
		database.ErrorHandling(sqlGetProductsPerSec, err, false)

		return
	}
	defer productRows.Close()
	productMap := make(map[int]models.ProductStruct, 0)

	for productRows.Next() {
		var productId int
		var timePerUnitInSec float64
		err = productRows.Scan(&productId, &timePerUnitInSec)

		if err != nil {
			database.ErrorHandling(sqlGetProductsPerSec, err, false)

			return
		}

		productMap[productId] = models.ProductStruct{ProductId: productId, TimePerProductUnitInSec: timePerUnitInSec}
	}

	zap.S().Debugf("AssetID: %d", workCellId)
	data = CalculateAccumulatedProducts(to, observationStart, observationEnd, countMap, orderMap, productMap)
	return data, nil
}
