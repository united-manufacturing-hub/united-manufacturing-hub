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
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/lib/pq"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
)

// GetLocations retrieves all locations for a given customer
func GetLocations(customerID string) (locations []string, err error) {
	zap.S().Infof("[GetLocations] customerID: %v", customerID)

	sqlStatement := `SELECT distinct(location) FROM assetTable WHERE customer=$1;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, customerID)
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
		var location string
		err = rows.Scan(&location)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}
		locations = append(locations, location)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return
}

// GetAssets retrieves all assets for a given customer
func GetAssets(customerID string, location string) (assets []string, err error) {
	zap.S().Infof("[GetAssets] customerID: %v, location: %v", customerID, location)

	sqlStatement := `SELECT distinct(assetID) FROM assetTable WHERE customer=$1 AND location=$2;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, customerID, location)
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
		var asset string
		err = rows.Scan(&asset)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		assets = append(assets, asset)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}

// GetComponents retrieves all assets for a given customer
func GetComponents(assetID uint32) (components []string, err error) {
	zap.S().Infof("[GetComponents] assetID: %v", assetID)

	sqlStatement := `SELECT distinct(componentname) FROM componentTable WHERE asset_id=$1;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID)
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
		var component string
		err = rows.Scan(&component)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		components = append(components, component)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}

// GetStatesRaw gets all states for a specific asset in a timerange. It returns an array of datamodel.StateEntry
func GetStatesRaw(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time,
	configuration datamodel.CustomerConfiguration) (data []datamodel.StateEntry, err error) {
	zap.S().Infof(
		"[GetStatesRaw] customerID: %v, location: %v, asset: %v, from: %v, to: %v, configuration: %v",
		customerID,
		location,
		asset,
		from,
		to,
		configuration)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	key := fmt.Sprintf("GetStatesRaw-%d-%s-%s-%s", assetID, from, to, internal.AsHash(configuration))
	if database.Mutex.TryLock(key) { // is is already running?
		defer database.Mutex.Unlock(key)

		// Get from cache if possible
		var cacheHit bool
		data, cacheHit = internal.GetStatesRawFromCache(assetID, from, to, configuration)
		if cacheHit { // data found
			zap.S().Debugf("GetStatesRaw cache hit")
			return
		}

		// no data in cache

		// Additionally, get the latest state before the time range
		var timestamp time.Time
		var dataPoint int

		sqlStatement := `SELECT timestamp, state FROM stateTable WHERE asset_id=$1 AND timestamp < $2 ORDER BY timestamp DESC LIMIT 1;`

		err = database.Db.QueryRow(sqlStatement, assetID, from).Scan(&timestamp, &dataPoint)
		if errors.Is(err, sql.ErrNoRows) {
			// it can happen, no need to escalate error
			zap.S().Debugf("No Results Found")
		} else if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		} else {
			// use "from" timestamp instead of timestamp in the state as we want to look only at data within the selected time range

			dataPoint = datamodel.ConvertOldToNew(dataPoint)

			fullRow := datamodel.StateEntry{
				State:     dataPoint,
				Timestamp: from,
			}
			data = append(data, fullRow)
		}

		sqlStatement = `SELECT timestamp, state FROM stateTable WHERE asset_id=$1 AND timestamp BETWEEN $2 AND $3 ORDER BY timestamp ASC;`

		var rows *sql.Rows
		rows, err = database.Db.Query(sqlStatement, assetID, from, to)
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
			var timestamp time.Time
			var dataPoint int

			err = rows.Scan(&timestamp, &dataPoint)
			if err != nil {
				database.ErrorHandling(sqlStatement, err, false)

				return
			}

			dataPoint = datamodel.ConvertOldToNew(dataPoint)

			fullRow := datamodel.StateEntry{
				State:     dataPoint,
				Timestamp: timestamp,
			}
			data = append(data, fullRow)
		}
		err = rows.Err()
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}

		internal.StoreRawStatesToCache(assetID, from, to, configuration, data)

	} else {
		zap.S().Error("Failed to get Mutex")
	}

	return
}

// GetShiftsRaw gets all shifts for a specific asset in a timerange in a raw format
func GetShiftsRaw(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time,
	configuration datamodel.CustomerConfiguration) (data []datamodel.ShiftEntry, err error) {
	zap.S().Infof(
		"[GetShiftsRaw] customerID: %v, location: %v, asset: %v, from: %v, to: %v, configuration: %v",
		customerID,
		location,
		asset,
		from,
		to,
		configuration)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {
		return
	}

	key := fmt.Sprintf("GetShiftsRaw-%d-%s-%s-%s", assetID, from, to, internal.AsHash(configuration))
	if database.Mutex.TryLock(key) { // is is already running?
		defer database.Mutex.Unlock(key)

		// Get from cache if possible
		var cacheHit bool
		data, cacheHit = internal.GetRawShiftsFromCache(assetID, from, to, configuration)
		if cacheHit { // data found
			zap.S().Debugf("GetShiftsRaw cache hit")
			return
		}

		// no data in cache

		var timestampStart time.Time
		var timestampEnd time.Time
		var shiftType int

		sqlStatement := `
		SELECT begin_timestamp, end_timestamp, type 
		FROM shiftTable 
		WHERE asset_id=$1 
			AND ((begin_timestamp BETWEEN $2 AND $3 OR end_timestamp BETWEEN $2 AND $3) 
			OR (begin_timestamp < $2 AND end_timestamp > $3))
		ORDER BY begin_timestamp ASC LIMIT 1;
		`

		err = database.Db.QueryRow(sqlStatement, assetID, from, to).Scan(&timestampStart, &timestampEnd, &shiftType)
		if errors.Is(err, sql.ErrNoRows) {
			// it can happen, no need to escalate error
			zap.S().Debugf("No Results Found")

			// First entry is always noShift
			fullRow := datamodel.ShiftEntry{
				TimestampBegin: internal.UnixEpoch,
				TimestampEnd:   from,
				ShiftType:      0,
			}
			data = append(data, fullRow)
		} else if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		} else {
			// First entry is always noShift
			fullRow := datamodel.ShiftEntry{
				TimestampBegin: internal.UnixEpoch,
				TimestampEnd:   timestampStart, // .Add(time.Duration(-1) * time.Millisecond)
				ShiftType:      0,
			}
			data = append(data, fullRow)

			// use "from" timestamp instead of timestamp in the state as we want to look only at data within the selected time range
			fullRow = datamodel.ShiftEntry{
				TimestampBegin: timestampStart,
				TimestampEnd:   timestampEnd,
				ShiftType:      shiftType,
			}
			data = append(data, fullRow)
		}

		sqlStatement = `
		SELECT begin_timestamp, end_timestamp, type 
		FROM shiftTable 
		WHERE asset_id=$1 
			AND ((begin_timestamp BETWEEN $2 AND $3 OR end_timestamp BETWEEN $2 AND $3) 
			OR (begin_timestamp < $2 AND end_timestamp > $3))
		ORDER BY begin_timestamp ASC OFFSET 1;`

		var rows *sql.Rows
		rows, err = database.Db.Query(sqlStatement, assetID, from, to) // OFFSET to prevent entering first result twice
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

			err = rows.Scan(&timestampStart, &timestampEnd, &shiftType)
			if err != nil {
				database.ErrorHandling(sqlStatement, err, false)
				return
			}
			fullRow := datamodel.ShiftEntry{
				TimestampBegin: timestampStart,
				TimestampEnd:   timestampEnd,
				ShiftType:      shiftType,
			}
			data = append(data, fullRow)
		}
		err = rows.Err()
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}

		// Last entry is always noShift
		if timestampEnd.Before(to) {
			fullRow := datamodel.ShiftEntry{
				TimestampBegin: timestampEnd,
				TimestampEnd:   to,
				ShiftType:      0,
			}
			data = append(data, fullRow)
		}

		internal.StoreRawShiftsToCache(assetID, from, to, configuration, data)
	} else {
		zap.S().Error("Failed to get Mutex")
	}

	return
}

// GetShifts gets all shifts for a specific asset in a timerange
func GetShifts(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetShiftsRaw] customerID: %v, location: %v, asset: %v, from: %v, to: %v",
		customerID,
		location,
		asset,
		from,
		to)

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "shiftName"
	data.ColumnNames = []string{"timestamp", JSONColumnName}

	var configuration datamodel.CustomerConfiguration
	configuration, err = GetCustomerConfiguration(customerID)
	if err != nil {
		zap.S().Errorw(
			"GetCustomerConfiguration failed",
			"error", err,
		)
		return
	}

	var rawShifts []datamodel.ShiftEntry
	rawShifts, err = GetShiftsRaw(customerID, location, asset, from, to, configuration)
	if err != nil {
		zap.S().Errorw(
			"GetShiftsRaw failed",
			"error", err,
		)
		return
	}

	processedShifts := cleanRawShiftData(rawShifts)
	processedShifts = addNoShiftsBetweenShifts(processedShifts)

	// Loop through all datapoints
	for _, dataPoint := range processedShifts {
		// TODO: #86 Return timestamps in RFC3339 in /shifts
		formattedBegin := dataPoint.TimestampBegin.Format(time.RFC3339) //formatting the Unix time to RFC3339
		fullRow := []interface{}{
			formattedBegin,
			dataPoint.ShiftType}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	return

}

// GetProcessValue gets all data for specific valueName and for a specific asset in a timerange
func GetProcessValue(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time,
	valueName string) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetShiftsRaw] customerID: %v, location: %v, asset: %v, from: %v, to: %v, valueName: %v",
		customerID,
		location,
		asset,
		from,
		to,
		valueName)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {
		return
	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + valueName

	data.ColumnNames = []string{"timestamp", JSONColumnName}

	sqlStatement := `SELECT timestamp, value FROM processValueTable WHERE asset_id=$1 AND (timestamp BETWEEN $2 AND $3) AND valueName=$4 ORDER BY timestamp ASC;`
	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID, from, to, valueName)
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

		var timestamp time.Time
		var dataPoint float64

		err = rows.Scan(&timestamp, &dataPoint)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		formatted := timestamp.Format(time.RFC3339) //formatting the Unix time to RFC3339
		fullRow := []interface{}{
			formatted,
			dataPoint}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return

}

func GetProcessValueString(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time,
	valueName string) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetProcessValueString] customerID: %v, location: %v, asset: %v, from: %v, to: %v, valueName: %v",
		customerID,
		location,
		asset,
		from,
		to,
		valueName)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + valueName

	data.ColumnNames = []string{"timestamp", JSONColumnName}

	sqlStatement := `SELECT timestamp, value FROM ProcessValueStringTable WHERE asset_id=$1 AND (timestamp BETWEEN $2 AND $3) AND valueName=$4 ORDER BY timestamp ASC;`
	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID, from, to, valueName)
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

		var timestamp time.Time
		var dataPoint string

		err = rows.Scan(&timestamp, &dataPoint)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}

		formatted := timestamp.Format(time.RFC3339) //formatting the Unix time to RFC3339
		fullRow := []interface{}{
			formatted,
			dataPoint}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return

}

// GetCurrentState gets the latest state of an asset
func GetCurrentState(
	customerID string,
	location string,
	asset string,
	keepStatesInteger bool) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetCurrentState] customerID: %v, location: %v, asset: %v keepStatesInteger: %v",
		customerID,
		location,
		asset,
		keepStatesInteger)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "state"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	var configuration datamodel.CustomerConfiguration
	configuration, err = GetCustomerConfiguration(customerID)
	if err != nil {
		zap.S().Errorw(
			"GetCustomerConfiguration failed",
			"error", err,
		)

		return
	}

	var timestamp time.Time
	var dataPoint int

	sqlStatement := `SELECT timestamp, state FROM stateTable WHERE asset_id=$1 ORDER BY timestamp DESC LIMIT 1;`
	err = database.Db.QueryRow(sqlStatement, assetID).Scan(&timestamp, &dataPoint)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	// Convert old data model to new data model
	dataPoint = datamodel.ConvertOldToNew(dataPoint)

	if keepStatesInteger {
		formatted := timestamp.Format(time.RFC3339) //formatting the Unix time to RFC3339
		fullRow := []interface{}{
			dataPoint,
			formatted}
		data.Datapoints = append(data.Datapoints, fullRow)
	} else {
		formatted := timestamp.Format(time.RFC3339) //formatting the Unix time to RFC3339
		fullRow := []interface{}{
			ConvertStateToString(dataPoint, configuration),
			formatted}
		data.Datapoints = append(data.Datapoints, fullRow)
	}

	return
}

// GetDataTimeRangeForAsset gets the first and latest timestamp. This is used to show all existing data e.g. to create recommendations
func GetDataTimeRangeForAsset(customerID string, location string, asset string) (
	data datamodel.DataResponseAny,
	err error) {
	zap.S().Infof("[GetDataTimeRangeForAsset] customerID: %v, location: %v, asset: %v", customerID, location, asset)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}
	data.ColumnNames = []string{"firstTimestamp", "lastTimestamp"}

	var lastTimestampPq pq.NullTime
	var firstTimestampPq pq.NullTime

	var lastTimestamp time.Time
	var firstTimestamp time.Time

	// last timestamp
	sqlStatement := `SELECT MAX(timestamp),MIN(timestamp) FROM stateTable WHERE asset_id=$1;`

	err = database.Db.QueryRow(sqlStatement, assetID).Scan(&lastTimestampPq, &firstTimestampPq)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	if !lastTimestampPq.Valid || !firstTimestampPq.Valid {
		err = errors.New("asset has no states yet")
		return
	}

	lastTimestamp = lastTimestampPq.Time
	firstTimestamp = firstTimestampPq.Time

	fullRow := []interface{}{firstTimestamp.Format(time.RFC3339), lastTimestamp.Format(time.RFC3339)}
	// fullRow := []float64{float64(firstTimestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))), float64(lastTimestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
	data.Datapoints = append(data.Datapoints, fullRow)

	return
}

// GetCountsRaw gets all states for a specific asset in a timerange
func GetCountsRaw(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time) (data []datamodel.CountEntry, err error) {
	zap.S().Infof(
		"[GetCountsRaw] customerID: %v, location: %v, asset: %v from: %v, to: %v",
		customerID,
		location,
		asset,
		from,
		to)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	key := fmt.Sprintf("GetCountsRaw-%d-%s-%s", assetID, from, to)
	if database.Mutex.TryLock(key) { // is is already running?
		defer database.Mutex.Unlock(key)

		// Get from cache if possible
		var cacheHit bool
		data, cacheHit = internal.GetRawCountsFromCache(assetID, from, to)
		if cacheHit { // data found
			zap.S().Debugf("GetCountsRaw cache hit")
			return
		}

		// no data in cache
		sqlStatement := `SELECT timestamp, count, scrap FROM countTable WHERE asset_id=$1 AND timestamp BETWEEN $2 AND $3 ORDER BY timestamp ASC;`
		var rows *sql.Rows
		rows, err = database.Db.Query(sqlStatement, assetID, from, to)
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
			var timestamp time.Time
			var count int32
			var scrapN sql.NullInt32

			err = rows.Scan(&timestamp, &count, &scrapN)
			if err != nil {
				database.ErrorHandling(sqlStatement, err, false)

				return
			}

			var scrap int32
			if scrapN.Valid {
				scrap = scrapN.Int32
			}

			fullRow := datamodel.CountEntry{
				Count:     float64(count),
				Scrap:     float64(scrap),
				Timestamp: timestamp,
			}
			data = append(data, fullRow)
		}
		err = rows.Err()
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}

		internal.StoreRawCountsToCache(assetID, from, to, data)
	} else {
		zap.S().Error("Failed to get Mutex")
	}

	return
}

// GetCounts gets all states for a specific asset in a timerange
func GetCounts(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetCounts] customerID: %v, location: %v, asset: %v from: %v, to: %v",
		customerID,
		location,
		asset,
		from,
		to)

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "count"
	JSONColumnName2 := customerID + "-" + location + "-" + asset + "-" + "scrap"
	data.ColumnNames = []string{JSONColumnName, JSONColumnName2, "timestamp"}

	var countSlice []datamodel.CountEntry
	countSlice, err = GetCountsRaw(customerID, location, asset, from, to)
	if err != nil {
		zap.S().Errorf("GetCountsRaw failed", err)

		return
	}

	// Loop through all datapoints
	for _, dataPoint := range countSlice {
		formatted := dataPoint.Timestamp.Format(time.RFC3339) //formatting the Unix time to RFC3339
		fullRow := []interface{}{
			dataPoint.Count,
			dataPoint.Scrap,
			formatted}
		data.Datapoints = append(data.Datapoints, fullRow)
	}

	return
}

// TODO test GetTotalCounts

/*
// GetTotalCounts gets the sum of produced units for a specific asset in a timerange
func GetTotalCounts(
	c *gin.Context,
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetTotalCounts] customerID: %v, location: %v, asset: %v from: %v, to: %v",
		customerID,
		location,
		asset,
		from,
		to)

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "count"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	countSlice, err = GetCountsRaw(c, customerID, location, asset, from, to)
	if err != nil {
		zap.S().Errorf("GetCountsRaw failed", err)

		return
	}

	var totalCount float64 = 0

	// Loop through all datapoints
	for _, dataPoint := range countSlice {
		totalCount = totalCount + dataPoint.Count
	}

	fullRow := []interface{}{totalCount, float64(to.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
	data.Datapoints = append(data.Datapoints, fullRow)

	return
}s
*/

// GetProductionSpeed gets the production speed (units/hour) in a selectable interval (in minutes) for a given time range
func GetProductionSpeed(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time,
	aggregatedInterval int) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetProductionSpeed] customerID: %v, location: %v, asset: %v from: %v, to: %v, aggregatedInterval: %v",
		customerID,
		location,
		asset,
		from,
		to,
		aggregatedInterval)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "speed"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	aggregatedIntervalString := "1 minutes" //default time interval
	speedInterval := float64(1)

	//aggrationInterval is non-required parameter
	if aggregatedInterval > 0 {
		aggregatedIntervalString = strconv.Itoa(aggregatedInterval) + " minutes"
		speedInterval = float64(aggregatedInterval)
	}

	// time_bucket_gapfill does not work on Microsoft Azure (license issue)
	sqlStatement := `
	SELECT time_bucket($1, timestamp) as speedPerIntervall, coalesce(sum(count),0)  
	FROM countTable 
	WHERE asset_id=$2 
		AND timestamp BETWEEN $3 AND $4 
	GROUP BY speedPerIntervall 
	ORDER BY speedPerIntervall ASC;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, aggregatedIntervalString, assetID, from, to)

	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	defer rows.Close()

	// for custom gapfilling
	var previousTimestamp time.Time

	for rows.Next() {
		var timestamp time.Time
		var dataPoint float64

		err = rows.Scan(&timestamp, &dataPoint)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}

		// TODO: #92 Return timestamps in RFC3339 in /productionSpeed

		// gapfilling to have constant 0 in grafana
		if !previousTimestamp.IsZero() {
			timeDifference := timestamp.Unix() - previousTimestamp.Unix()

			if timeDifference > 60 { // bigger than one minute
				t1 := previousTimestamp.Add(time.Second * 60) // 60 = adding 60 seconds
				formatted1 := t1.Format(time.RFC3339)         //formatting the Unix time to RFC3339

				t2 := timestamp.Add(-time.Second * 1) // -1 = subtracting one s
				formatted2 := t2.Format(time.RFC3339) //formatting the Unix time to RFC3339

				// add zero speed one minute after previous timestamp
				fullRow := []interface{}{
					0,
					formatted1}
				data.Datapoints = append(data.Datapoints, fullRow)

				// add zero speed one ms before timestamp
				fullRow = []interface{}{
					0,
					formatted2}
				data.Datapoints = append(data.Datapoints, fullRow)
			}
		}
		// add datapoint
		formatted3 := timestamp.Format(time.RFC3339) //formatting the Unix time to RFC3339
		fullRow := []interface{}{
			dataPoint * 60 / speedInterval,
			formatted3} // *60 / speed Interval to get the production speed per hour
		data.Datapoints = append(data.Datapoints, fullRow)

		previousTimestamp = timestamp
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return
}

// GetQualityRate gets the quality rate in a selectable interval (in minutes) for a given time range
func GetQualityRate(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time,
	aggregatedInterval int) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetQualityRate] customerID: %v, location: %v, asset: %v from: %v, to: %v, aggregatedInterval: %v",
		customerID,
		location,
		asset,
		from,
		to,
		aggregatedInterval)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "speed"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	aggregatedIntervalString := "1 minutes" //default time interval

	//aggrationInterval is non-required parameter
	if aggregatedInterval > 0 {
		aggregatedIntervalString = strconv.Itoa(aggregatedInterval) + " minutes"
	}

	// time_bucket_gapfill does not work on Microsoft Azure (license issue)
	sqlStatement := `
	SELECT time_bucket($1, timestamp) as ratePerIntervall, 
		coalesce(
			(sum(count)-sum(scrap))::float / sum(count)
		,1)
	FROM countTable 
	WHERE asset_id=$2 
		AND timestamp BETWEEN $3 AND $4 
	GROUP BY ratePerIntervall 
	ORDER BY ratePerIntervall ASC;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, aggregatedIntervalString, assetID, from, to)

	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	defer rows.Close()

	// for custom gapfilling
	var previousTimestamp time.Time

	for rows.Next() {
		var timestamp time.Time
		var dataPoint float64

		err = rows.Scan(&timestamp, &dataPoint)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}

		// TODO: Return timestamps in RFC3339 in /qualityRate

		// gapfilling to have constant 0 in grafana
		if !previousTimestamp.IsZero() {
			timeDifference := timestamp.Unix() - previousTimestamp.Unix()

			if timeDifference > 60 { // bigger than one minute
				// add 100% quality one minute after previous timestamp
				fullRow := []interface{}{
					1,
					previousTimestamp.Add(time.Second * 60).Format(time.RFC3339)} // 60 = adding 60 seconds
				data.Datapoints = append(data.Datapoints, fullRow)

				// add 100% one ms before timestamp
				fullRow = []interface{}{
					1,
					timestamp.Add(-time.Second * 1).Format(time.RFC3339)} // -1 = subtracting one s
				data.Datapoints = append(data.Datapoints, fullRow)
			}
		}
		// add datapoint
		fullRow := []interface{}{
			dataPoint,
			timestamp.Format(time.RFC3339)}
		data.Datapoints = append(data.Datapoints, fullRow)

		previousTimestamp = timestamp
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return
}

// GetCustomerConfiguration fetches the customer configuration (KPI definition, etc.) from the database
func GetCustomerConfiguration(customerID string) (configuration datamodel.CustomerConfiguration, err error) {
	zap.S().Infof("[GetCustomerConfiguration] customerID: %v", customerID)

	// Get from cache if possible
	var cacheHit bool
	configuration, cacheHit = internal.GetCustomerConfigurationFromCache(customerID)
	if cacheHit { // data found
		return
	}

	tempAvailabilityLossStates := pq.Int32Array{}
	tempPerformanceLossStates := pq.Int32Array{}

	sqlStatement := `
		SELECT 
			MicrostopDurationInSeconds, 
			IgnoreMicrostopUnderThisDurationInSeconds,
			MinimumRunningTimeInSeconds,
			ThresholdForNoShiftsConsideredBreakInSeconds,
			LowSpeedThresholdInPcsPerHour,
			AutomaticallyIdentifyChangeovers,
			LanguageCode,
			AvailabilityLossStates,
			PerformanceLossStates
		FROM 
			configurationTable 
		WHERE 
			customer=$1;
	`
	err = database.Db.QueryRow(sqlStatement, customerID).Scan(
		&configuration.MicrostopDurationInSeconds,
		&configuration.IgnoreMicrostopUnderThisDurationInSeconds,
		&configuration.MinimumRunningTimeInSeconds,
		&configuration.ThresholdForNoShiftsConsideredBreakInSeconds,
		&configuration.LowSpeedThresholdInPcsPerHour,
		&configuration.AutomaticallyIdentifyChangeovers,
		&configuration.LanguageCode,
		&tempAvailabilityLossStates,
		&tempPerformanceLossStates,
	)

	if errors.Is(err, sql.ErrNoRows) { // default values if no configuration is stored yet
		configuration.MicrostopDurationInSeconds = 60 * 2
		configuration.IgnoreMicrostopUnderThisDurationInSeconds = -1 // do not apply
		configuration.MinimumRunningTimeInSeconds = 0
		configuration.ThresholdForNoShiftsConsideredBreakInSeconds = 60 * 35
		configuration.LowSpeedThresholdInPcsPerHour = -1 // do not apply by default
		configuration.AutomaticallyIdentifyChangeovers = true
		configuration.AvailabilityLossStates = append(
			configuration.AvailabilityLossStates,
			40000,
			180000,
			190000,
			200000,
			210000,
			220000)
		configuration.PerformanceLossStates = append(
			configuration.PerformanceLossStates,
			20000,
			50000,
			60000,
			70000,
			80000,
			90000,
			100000,
			110000,
			120000,
			130000,
			140000,
			150000)
		configuration.LanguageCode = datamodel.LanguageEnglish // English
		zap.S().Warnf("No configuration stored for customer %s, using default !", customerID)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	configuration.AvailabilityLossStates = tempAvailabilityLossStates
	configuration.PerformanceLossStates = tempPerformanceLossStates

	// Store to cache if not yet existing
	go internal.StoreCustomerConfigurationToCache(customerID, configuration)
	zap.S().Debug("Stored configuration to cache")

	return
}

// GetRecommendations gets all current recommendations for a specific asset
func GetRecommendations(customerID string, location string, asset string) (
	data datamodel.DataResponseAny,
	err error) {
	zap.S().Infof("[GetRecommendations] customerID: %v, location: %v, asset: %v", customerID, location, asset)

	data.ColumnNames = []string{
		"timestamp",
		"recommendationType",
		"recommendationValues",
		"recommendationTextEN",
		"recommendationTextDE",
		"diagnoseTextEN",
		"diagnoseTextDE"}

	var likeString string = customerID + "-" + location + "-" + asset + "-%"
	sqlStatement := `
	SELECT timestamp, recommendationType, recommendationValues, recommendationTextEN, recommendationTextDE, diagnoseTextEN, diagnoseTextDE 
	FROM recommendationTable 
	WHERE enabled=True 
		AND uid LIKE $1 
		AND (timestamp = (SELECT MAX(timestamp) FROM recommendationTable WHERE enabled=True AND uid LIKE $1)) 
	ORDER BY timestamp DESC;`
	// AND (timestamp=) used to only get the recommendations from the latest calculation batch (avoid showing old ones)
	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, likeString)
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
		var timestamp time.Time
		var recommendationType int
		var recommendationValues string
		var diagnoseTextEN string
		var diagnoseTextDE string
		var recommendationTextEN string
		var recommendationTextDE string

		err = rows.Scan(
			&timestamp,
			&recommendationType,
			&recommendationValues,
			&recommendationTextEN,
			&recommendationTextDE,
			&diagnoseTextEN,
			&diagnoseTextDE)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}

		// TODO: #87 Return timestamps in RFC3339 in /recommendations
		unixTimestamp4 := timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
		t4 := time.Unix(unixTimestamp4, 0)
		formatted4 := t4.Format(time.RFC3339) //formatting the Unix time to RFC3339
		fullRow := []interface{}{
			formatted4,
			recommendationType,
			recommendationValues,
			recommendationTextEN,
			recommendationTextDE,
			diagnoseTextEN,
			diagnoseTextDE}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return
}

// GetMaintenanceActivities gets all maintenance activities for a specific asset
func GetMaintenanceActivities(customerID string, location string, asset string) (
	data datamodel.DataResponseAny,
	err error) {
	zap.S().Infof("[GetMaintenanceActivities] customerID: %v, location: %v, asset: %v", customerID, location, asset)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}
	data.ColumnNames = []string{"Component", "Activity", "Timestamp"}

	sqlStatement := `
	SELECT componentname, activitytype, timestamp 
	FROM maintenanceActivities 
	INNER JOIN componentTable ON 
		(maintenanceActivities.component_id = componentTable.id) 
	WHERE component_id IN (SELECT component_id FROM componentTable WHERE asset_id = $1);`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID)
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

		var componentName string
		var activityType int
		var timestamp time.Time

		err = rows.Scan(&componentName, &activityType, &timestamp)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}
		fullRow := []interface{}{
			componentName,
			activityType,
			float64(timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return
}

// GetUniqueProducts gets all unique products for a specific asset in a specific time range
func GetUniqueProducts(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetUniqueProducts] customerID: %v, location: %v, asset: %v from: %v, to: %v",
		customerID,
		location,
		asset,
		from,
		to)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}
	data.ColumnNames = []string{"UID", "AID", "TimestampBegin", "TimestampEnd", "ProductID", "IsScrap"}

	sqlStatement := `
	SELECT uniqueProductID, uniqueProductAlternativeID, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap 
	FROM uniqueProductTable 
	WHERE asset_id = $1 
		AND (begin_timestamp_ms BETWEEN $2 AND $3 OR end_timestamp_ms BETWEEN $2 AND $3) 
		OR (begin_timestamp_ms < $2 AND end_timestamp_ms > $3) 
	ORDER BY begin_timestamp_ms ASC;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID, from, to)
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

// GetUpcomingTimeBasedMaintenanceActivities returns UpcomingTimeBasedMaintenanceActivities array for an asset
func GetUpcomingTimeBasedMaintenanceActivities(
	customerID string,
	location string,
	asset string) (data []datamodel.UpcomingTimeBasedMaintenanceActivities, err error) {
	zap.S().Infof(
		"[GetUpcomingTimeBasedMaintenanceActivities] customerID: %v, location: %v, asset: %v",
		customerID,
		location,
		asset)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	sqlStatement := `
	SELECT 
	componentTable.componentname,
	timebasedmaintenance.intervallinhours, 
	timebasedmaintenance.activitytype, 
	uniqMaintAct.timestamp as latest_activity, 
	uniqMaintAct.timestamp + (timebasedmaintenance.intervallinhours || ' hour')::interval as next_activity,
	round((EXTRACT(EPOCH FROM uniqMaintAct.timestamp + (timebasedmaintenance.intervallinhours || ' hour')::interval - NOW())/60/60/24) ::numeric, 2) as duration

	FROM timebasedmaintenance 
		FULL JOIN 
			(
				SELECT * FROM maintenanceActivities 
				WHERE (component_id, activitytype, timestamp) IN 
					(
						SELECT component_id, activitytype, MAX(timestamp) 
						FROM maintenanceActivities 
						GROUP BY component_id, activitytype
					)
			) uniqMaintAct 
			ON uniqMaintAct.component_id = timebasedmaintenance.component_id AND uniqMaintAct.activitytype = timebasedmaintenance.activitytype
		FULL JOIN componentTable ON (timebasedmaintenance.component_id = componentTable.id)
			
	WHERE timebasedmaintenance.component_id IN (SELECT timebasedmaintenance.component_id FROM componentTable WHERE asset_id = $1);
	`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID)
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

		var ComponentName string
		var IntervallInHours int
		var ActivityType int

		var LatestActivity pq.NullTime
		var NextActivity pq.NullTime
		var Duration sql.NullFloat64

		err = rows.Scan(&ComponentName, &IntervallInHours, &ActivityType, &LatestActivity, &NextActivity, &Duration)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}
		fullRow := datamodel.UpcomingTimeBasedMaintenanceActivities{
			ComponentName:    ComponentName,
			IntervallInHours: IntervallInHours,
			ActivityType:     ActivityType,
			LatestActivity:   LatestActivity,
			NextActivity:     NextActivity,
			DurationInDays:   Duration,
		}

		data = append(data, fullRow)
	}
	return
}

// GetOrdersRaw gets all order and product infirmation in a specific time range for an asset
func GetOrdersRaw(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time) (data []datamodel.OrdersRaw, err error) {
	zap.S().Infof(
		"[GetOrdersRaw] customerID: %v, location: %v, asset: %v from: %v, to: %v",
		customerID,
		location,
		asset,
		from,
		to)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	sqlStatement := `
		SELECT order_name, target_units, begin_timestamp, end_timestamp, productTable.product_name, productTable.time_per_unit_in_seconds
		FROM orderTable 
		FULL JOIN productTable ON productTable.product_id = orderTable.product_id
		WHERE 
			begin_timestamp IS NOT NULL 
			AND end_timestamp IS NOT NULL 
			AND orderTable.asset_id = $1 
			AND ( 
				(begin_timestamp BETWEEN $2 AND $3 OR end_timestamp BETWEEN $2 AND $3) 
				OR (begin_timestamp < $2 AND end_timestamp > $3)
			)
		ORDER BY begin_timestamp ASC;
	`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID, from, to)
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

		var orderName string
		var targetUnits int

		var beginTimestamp time.Time
		var endTimestamp time.Time

		var productName string
		var timePerUnitInSeconds float64

		err = rows.Scan(&orderName, &targetUnits, &beginTimestamp, &endTimestamp, &productName, &timePerUnitInSeconds)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}
		fullRow := datamodel.OrdersRaw{
			OrderName:            orderName,
			TargetUnits:          targetUnits,
			BeginTimestamp:       beginTimestamp,
			EndTimestamp:         endTimestamp,
			ProductName:          productName,
			TimePerUnitInSeconds: timePerUnitInSeconds,
		}
		data = append(data, fullRow)
	}
	return
}

// GetUnstartedOrdersRaw gets all order and product infirmation for an asset that have not started yet
func GetUnstartedOrdersRaw(customerID string, location string, asset string) (
	data []datamodel.OrdersUnstartedRaw,
	err error) {
	zap.S().Infof("[GetUnstartedOrdersRaw] customerID: %v, location: %v, asset: %v", customerID, location, asset)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	sqlStatement := `
		SELECT order_name, target_units, productTable.product_name, productTable.time_per_unit_in_seconds
		FROM orderTable 
		FULL JOIN productTable ON productTable.product_id = orderTable.product_id
		WHERE 
			begin_timestamp IS NULL 
			AND end_timestamp IS NULL 
			AND orderTable.asset_id = $1;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID)
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

		var orderName string
		var targetUnits int

		var productName string
		var timePerUnitInSeconds float64

		err = rows.Scan(&orderName, &targetUnits, &productName, &timePerUnitInSeconds)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}
		fullRow := datamodel.OrdersUnstartedRaw{
			OrderName:            orderName,
			TargetUnits:          targetUnits,
			ProductName:          productName,
			TimePerUnitInSeconds: timePerUnitInSeconds,
		}
		data = append(data, fullRow)
	}
	return
}

// GetDistinctProcessValues gets all possible process values for a specific asset. It returns an array of strings with every string starting with process_
func GetDistinctProcessValues(customerID string, location string, asset string) (data []string, err error) {
	zap.S().Infof("[GetDistinctProcessValues] customerID: %v, location: %v, asset: %v", customerID, location, asset)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	sqlStatement := `SELECT distinct valueName FROM processValueTable WHERE asset_id=$1;`
	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID)
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
		var currentString string

		err = rows.Scan(&currentString)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}

		data = append(data, "process_"+currentString)
	}

	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return
}

func GetDistinctProcessValuesString(customerID string, location string, asset string) (data []string, err error) {
	zap.S().Infof(
		"[GetDistinctProcessValuesString] customerID: %v, location: %v, asset: %v",
		customerID,
		location,
		asset)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	sqlStatement := `SELECT distinct valueName FROM processvaluestringtable WHERE asset_id=$1;`
	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, assetID)
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
		var currentString string

		err = rows.Scan(&currentString)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}

		data = append(data, "processString_"+currentString)
	}

	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return
}

// GetAssetID gets the assetID from the database
func GetAssetID(customerID string, location string, assetID string) (DBassetID uint32, err error) {
	zap.S().Infof("[GetAssetID] customerID: %v, location: %v, assetID: %v", customerID, location, assetID)

	// Get from cache if possible
	var cacheHit bool
	DBassetID, cacheHit = internal.GetAssetIDFromCache(customerID, location, assetID)
	if cacheHit { // data found
		// zap.S().Debugf("GetAssetID cache hit")
		return
	}

	sqlStatement := "SELECT id FROM assetTable WHERE assetid=$1 AND location=$2 AND customer=$3;"
	err = database.Db.QueryRow(sqlStatement, assetID, location, customerID).Scan(&DBassetID)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf(
			"[GetAssetID] No asset found for customerID: %v, location: %v, assetID: %v",
			customerID,
			location,
			assetID)
		err = errors.New("asset does not exist")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	// Store to cache if not yet existing
	go internal.StoreAssetIDToCache(customerID, location, assetID, DBassetID)
	zap.S().Debug("Stored AssetID to cache")

	return
}

// GetUniqueProductsWithTags gets all unique products with tags and parents for a specific asset in a specific time range
func GetUniqueProductsWithTags(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetUniqueProductsWithTags] customerID: %v, location: %v, asset: %v from: %v, to: %v",
		customerID,
		location,
		asset,
		from,
		to)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

	// getting all uniqueProducts and if existing all productTags (float)
	sqlStatementData := `
	SELECT uniqueProductID, uniqueProductAlternativeID, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap, valueName, value
	FROM uniqueProductTable 
		LEFT JOIN productTagTable ON uniqueProductTable.uniqueProductID = productTagTable.product_uid
	WHERE asset_id = $1 
		AND (begin_timestamp_ms BETWEEN $2 AND $3 OR end_timestamp_ms BETWEEN $2 AND $3) 
		OR (begin_timestamp_ms < $2 AND end_timestamp_ms > $3) 
	ORDER BY uniqueProductID ASC;`

	// getting productTagString (string) data linked to UID's
	sqlStatementDataStrings := `
	SELECT uniqueProductID, begin_timestamp_ms, valueName, value
	FROM uniqueProductTable 
		INNER JOIN productTagStringTable ON uniqueProductTable.uniqueProductID = productTagStringTable.product_uid
	WHERE asset_id = $1 
		AND (begin_timestamp_ms BETWEEN $2 AND $3 OR end_timestamp_ms BETWEEN $2 AND $3) 
		OR (begin_timestamp_ms < $2 AND end_timestamp_ms > $3) 
	ORDER BY uniqueProductID ASC;`

	// getting inheritance data (product_name and AID of parents at the specified asset)
	sqlStatementDataInheritance := `
	SELECT unProdTab.uniqueProductID, unProdTab.begin_timestamp_ms, prodTab.product_name, unProdTabForAID.uniqueProductAlternativeID
	FROM uniqueProductTable unProdTab
		INNER JOIN productInheritanceTable prodInher ON unProdTab.uniqueProductID = prodInher.child_uid
		INNER JOIN uniqueProductTable unProdTabForAID ON prodInher.parent_uid = unProdTabForAID.uniqueProductID
		INNER JOIN productTable prodTab ON unProdTabForAID.product_id = prodTab.product_id
	WHERE unProdTab.asset_id = $1 
		AND (unProdTab.begin_timestamp_ms BETWEEN $2 AND $3 OR unProdTab.end_timestamp_ms BETWEEN $2 AND $3) 
		OR (unProdTab.begin_timestamp_ms < $2 AND unProdTab.end_timestamp_ms > $3) 
	ORDER BY unProdTab.uniqueProductID ASC;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatementData, assetID, from, to)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatementData, err, false)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatementData, err, false)

		return
	}

	defer rows.Close()

	var rowsStrings *sql.Rows
	rowsStrings, err = database.Db.Query(sqlStatementDataStrings, assetID, from, to)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatementDataStrings, err, false)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatementDataStrings, err, false)

		return
	}

	defer rowsStrings.Close()

	var rowsInheritance *sql.Rows
	rowsInheritance, err = database.Db.Query(sqlStatementDataInheritance, assetID, from, to)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatementDataInheritance, err, false)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatementDataInheritance, err, false)

		return
	}

	defer rowsInheritance.Close()

	// Defining the base column names
	data.ColumnNames = []string{"UID", "AID", "timestamp", "timestampEnd", "ProductID", "IsScrap"}

	var indexRow int
	var indexColumn int

	// Rows can contain valueName and value or not: if not, they contain null
	for rows.Next() {

		var UID int
		var AID string
		var timestampBegin time.Time
		var timestampEnd sql.NullTime
		var productID int
		var isScrap bool
		var valueName sql.NullString
		var value sql.NullFloat64

		err = rows.Scan(&UID, &AID, &timestampBegin, &timestampEnd, &productID, &isScrap, &valueName, &value)
		if err != nil {
			database.ErrorHandling(sqlStatementData, err, false)

			return
		}

		// if productTag valueName not in data.ColumnNames yet (because the valueName of productTag comes up for the first time
		// in the current row), add valueName to data.ColumnNames, store index of column for data.DataPoints and extend slice
		if valueName.Valid {
			data.Datapoints, data.ColumnNames, indexColumn = ChangeOutputFormat(
				data.Datapoints,
				data.ColumnNames,
				valueName.String)
		}

		if data.Datapoints == nil { // if no row in data.Datapoints, create new row
			data.Datapoints = CreateNewRowInData(
				data.Datapoints, data.ColumnNames, indexColumn, UID, AID,
				timestampBegin, timestampEnd, productID, isScrap, valueName, value)
		} else { // if there are already rows in Data.datapoint
			indexRow = len(data.Datapoints) - 1
			lastUID, ok := data.Datapoints[indexRow][0].(int)
			if !ok {
				zap.S().Errorf("GetUniqueProductsWithTags: casting lastUID to int error", UID, timestampBegin)
				return
			}
			// check if the last row of data.Datapoints already has the same UID, as the current row, and if the
			// productTag information of the current row is valid. If yes: add productTag information of current row to
			// data.Datapoints
			if UID == lastUID && value.Valid && valueName.Valid {
				data.Datapoints[indexRow][indexColumn] = value.Float64
			} else if UID == lastUID && (!value.Valid || !valueName.Valid) { // if there are multiple lines with the same UID, each line should have a correct productTag
				zap.S().Errorf(
					"GetUniqueProductsWithTags: value.Valid or valueName.Valid false where it shouldn't",
					UID,
					timestampBegin)
				return
			} else if UID != lastUID { // create new row in tempDataPoints
				data.Datapoints = CreateNewRowInData(
					data.Datapoints, data.ColumnNames, indexColumn, UID, AID,
					timestampBegin, timestampEnd, productID, isScrap, valueName, value)
			} else {
				zap.S().Errorf("GetUniqueProductsWithTags: logic error", UID, timestampBegin)
				return
			}
		}

	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatementData, err, false)

		return
	}

	// all queried values should always exist here
	for rowsStrings.Next() {
		var UID int
		var timestampBegin time.Time
		var valueName sql.NullString
		var value sql.NullString

		err = rowsStrings.Scan(&UID, &timestampBegin, &valueName, &value)
		if err != nil {
			database.ErrorHandling(sqlStatementData, err, false)

			return
		}
		// Because of the inner join and the not null constraints of productTagString information in the postgresDB, both
		// valueName and value should be valid
		if !valueName.Valid || !value.Valid {
			zap.S().Errorf(
				"GetUniqueProductsWithTags: valueName or value for productTagString not valid",
				UID,
				timestampBegin)
			return
		}
		// if productTagString name not yet known, add to data.ColumnNames, store index for data.DataPoints in newColumns and extend slice
		data.Datapoints, data.ColumnNames, indexColumn = ChangeOutputFormat(
			data.Datapoints,
			data.ColumnNames,
			valueName.String)
		var contains bool // indicates, if the UID is already contained in the data.Datpoints slice or not
		contains, indexRow = SliceContainsInt(data.Datapoints, UID, 0)

		if contains { // true if UID already in data.Datapoints
			data.Datapoints[indexRow][indexColumn] = value.String
		} else { // throw error
			zap.S().Errorf("GetUniqueProductsWithTags: UID not found: Error!", UID, timestampBegin)
			return
		}
	}
	err = rowsStrings.Err()
	if err != nil {
		database.ErrorHandling(sqlStatementDataStrings, err, false)

		return
	}

	// all queried values should always exist here
	for rowsInheritance.Next() {
		var UID int
		var timestampBegin time.Time
		var productName string
		var AID string

		err = rowsInheritance.Scan(&UID, &timestampBegin, &productName, &AID)
		if err != nil {
			database.ErrorHandling(sqlStatementData, err, false)

			return
		}

		// if productName (describing type of product) not yet known, add to data.ColumnNames, store index for data.DataPoints in newColumns and extend slice
		data.Datapoints, data.ColumnNames, indexColumn = ChangeOutputFormat(
			data.Datapoints,
			data.ColumnNames,
			productName)
		var contains bool
		contains, indexRow = SliceContainsInt(data.Datapoints, UID, 0)

		if contains { // true if UID already in data.Datapoints
			data.Datapoints[indexRow][indexColumn] = AID
		} else {
			zap.S().Errorf("GetUniqueProductsWithTags: UID not found: Error!", UID, timestampBegin)
			return
		}
	}
	err = rowsInheritance.Err()
	if err != nil {
		database.ErrorHandling(sqlStatementDataInheritance, err, false)

		return
	}

	// CheckOutputDimensions checks, if the length of columnNames corresponds to the length of each row of data
	err = CheckOutputDimensions(data.Datapoints, data.ColumnNames)
	if err != nil {

		return
	}

	return
}

type CountStruct struct {
	timestamp time.Time
	count     int
	scrap     int
}
type OrderStruct struct {
	beginTimeStamp time.Time
	endTimeStamp   sql.NullTime
	orderID        int
	productId      int
	targetUnits    int
}

type ProductStruct struct {
	productId               int
	timePerProductUnitInSec float64
}

// GetAccumulatedProducts gets the accumulated counts for an observation timeframe and an asset
func GetAccumulatedProducts(
	customerID string,
	location string,
	asset string,
	from time.Time,
	to time.Time) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof(
		"[GetUniqueProductsWithTags] customerID: %v, location: %v, asset: %v from: %v, to: %v",
		customerID,
		location,
		asset,
		from,
		to)

	var assetID uint32
	assetID, err = GetAssetID(customerID, location, asset)
	if err != nil {

		return
	}

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
	row := database.Db.QueryRow(sqlStatementGetOutsider, assetID, from)
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
		insideOrderRows, err = database.Db.Query(sqlStatementGetInsiders, assetID, from, to, OuterOrder.OID)
	} else {
		// Get insiders
		zap.S().Debugf("Query without outsider: ", OuterOrder)
		insideOrderRows, err = database.Db.Query(sqlStatementGetInsidersNoOutsider, assetID, from, to)
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
		assetID,
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

	countMap := make([]CountStruct, 0)

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

		countMap = append(countMap, CountStruct{timestamp: timestamp, count: int(count), scrap: int(scrap)})
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
		assetID,
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

	orderMap := make([]OrderStruct, 0)

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
			orderMap, OrderStruct{
				orderID:        orderID,
				productId:      productId,
				targetUnits:    targetUnits,
				beginTimeStamp: beginTimeStamp,
				endTimeStamp:   endTimeStamp,
			})
	}

	sqlGetProductsPerSec := `SELECT product_id, time_per_unit_in_seconds FROM producttable WHERE asset_id = $1`

	var productRows *sql.Rows
	productRows, err = database.Db.Query(sqlGetProductsPerSec, assetID)

	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlGetProductsPerSec, err, false)
		return
	} else if err != nil {
		database.ErrorHandling(sqlGetProductsPerSec, err, false)

		return
	}
	defer productRows.Close()
	productMap := make(map[int]ProductStruct, 0)

	for productRows.Next() {
		var productId int
		var timePerUnitInSec float64
		err = productRows.Scan(&productId, &timePerUnitInSec)

		if err != nil {
			database.ErrorHandling(sqlGetProductsPerSec, err, false)

			return
		}

		productMap[productId] = ProductStruct{productId: productId, timePerProductUnitInSec: timePerUnitInSec}
	}

	zap.S().Debugf("AssetID: %d", assetID)
	data = CalculateAccumulatedProducts(to, observationStart, observationEnd, countMap, orderMap, productMap)
	return data, nil
}

// AfterOrEqual returns if t is after or equal to u
func AfterOrEqual(t time.Time, u time.Time) bool {
	return t.After(u) || t.Equal(u)
}
