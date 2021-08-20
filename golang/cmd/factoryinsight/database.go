package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/EagleChen/mapmutex"
	"github.com/omeid/pgerror"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"github.com/lib/pq"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
)

var db *sql.DB

// for caching
var mutex *mapmutex.Mutex

// SetupDB setups the db and stores the handler in a global variable in database.go
func SetupDB(PQUser string, PQPassword string, PWDBName string, PQHost string, PQPort int) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=require", PQHost, PQPort, PQUser, PQPassword, PWDBName)
	var err error
	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}

	mutex = mapmutex.NewCustomizedMapMutex(800, 100000000, 10, 1.1, 0.2) // default configs: maxDelay:  100000000, // 0.1 second baseDelay: 10,        // 10 nanosecond
}

// ShutdownDB closes all database connections
func ShutdownDB() {
	err := db.Close()
	if err != nil {
		panic(err)
	}
}

// PQErrorHandling logs and handles postgresql errors
func PQErrorHandling(parentSpan opentracing.Span, sqlStatement string, err error, isCritical bool) {

	ext.DBStatement.Set(parentSpan, sqlStatement)
	ext.DBType.Set(parentSpan, "sql")
	ext.LogError(parentSpan, err)

	traceID, _ := internal.ExtractTraceID(parentSpan)

	if e := pgerror.ConnectionException(err); e != nil {
		zap.S().Errorw("PostgreSQL failed: ConnectionException",
			"error", err,
			"sqlStatement", sqlStatement,
			"traceID", traceID)
		isCritical = true
	} else {
		zap.S().Errorw("PostgreSQL failed. ",
			"error", err,
			"sqlStatement", sqlStatement,
			"traceID", traceID)
	}

	if isCritical {
		ShutdownApplicationGraceful()
	}
}

// GetLocations retrieves all locations for a given customer
func GetLocations(parentSpan opentracing.Span, customerID string) (locations []string, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetLocations",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)

	sqlStatement := `SELECT distinct(location) FROM assetTable WHERE customer=$1;`

	rows, err := db.Query(sqlStatement, customerID)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	defer rows.Close()

	for rows.Next() {
		var location string
		err := rows.Scan(&location)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}
		locations = append(locations, location)
	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return
}

// GetAssets retrieves all assets for a given customer
func GetAssets(parentSpan opentracing.Span, customerID string, location string) (assets []string, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetAssets",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)

	sqlStatement := `SELECT distinct(assetID) FROM assetTable WHERE customer=$1 AND location=$2;`

	rows, err := db.Query(sqlStatement, customerID, location)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	defer rows.Close()

	for rows.Next() {
		var asset string
		err := rows.Scan(&asset)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}
		assets = append(assets, asset)
	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return
}

// GetComponents retrieves all assets for a given customer
func GetComponents(parentSpan opentracing.Span, assetID int) (components []string, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetComponents",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("assetID", assetID)

	sqlStatement := `SELECT distinct(componentname) FROM componentTable WHERE asset_id=$1;`

	rows, err := db.Query(sqlStatement, assetID)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	defer rows.Close()

	for rows.Next() {
		var component string
		err := rows.Scan(&component)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}
		components = append(components, component)
	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return
}

// GetStatesRaw gets all states for a specific asset in a timerange. It returns an array of datamodel.StateEntry
func GetStatesRaw(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (data []datamodel.StateEntry, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetStatesRaw",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}

	key := fmt.Sprintf("GetStatesRaw-%d-%s-%s-%s", assetID, from, to, internal.AsHash(configuration))
	if mutex.TryLock(key) { // is is already running?
		defer mutex.Unlock(key)

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

		err := db.QueryRow(sqlStatement, assetID, from).Scan(&timestamp, &dataPoint)
		if err == sql.ErrNoRows {
			// it can happen, no need to escalate error
			zap.S().Debugf("No Results Found")
		} else if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
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

		rows, err := db.Query(sqlStatement, assetID, from, to)
		if err == sql.ErrNoRows {
			PQErrorHandling(span, sqlStatement, err, false)
			return
		} else if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}

		defer rows.Close()

		for rows.Next() {
			var timestamp time.Time
			var dataPoint int

			err := rows.Scan(&timestamp, &dataPoint)
			if err != nil {
				PQErrorHandling(span, sqlStatement, err, false)
				error = err
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
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}

		internal.StoreRawStatesToCache(assetID, from, to, configuration, data)

	} else {
		zap.S().Error("Failed to get Mutex")
	}

	return
}

// GetShiftsRaw gets all shifts for a specific asset in a timerange in a raw format
func GetShiftsRaw(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (data []datamodel.ShiftEntry, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetShiftsRaw",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}

	key := fmt.Sprintf("GetShiftsRaw-%d-%s-%s-%s", assetID, from, to, internal.AsHash(configuration))
	if mutex.TryLock(key) { // is is already running?
		defer mutex.Unlock(key)

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

		err := db.QueryRow(sqlStatement, assetID, from, to).Scan(&timestampStart, &timestampEnd, &shiftType)
		if err == sql.ErrNoRows {
			// it can happen, no need to escalate error
			zap.S().Debugf("No Results Found")

			// First entry is always noShift
			fullRow := datamodel.ShiftEntry{
				TimestampBegin: time.Time{},
				TimestampEnd:   from,
				ShiftType:      0,
			}
			data = append(data, fullRow)
		} else if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		} else {
			// First entry is always noShift
			fullRow := datamodel.ShiftEntry{
				TimestampBegin: time.Time{},
				TimestampEnd:   timestampStart, //.Add(time.Duration(-1) * time.Millisecond)
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

		rows, err := db.Query(sqlStatement, assetID, from, to) //OFFSET to prevent entering first result twice
		if err == sql.ErrNoRows {
			PQErrorHandling(span, sqlStatement, err, false)
			return
		} else if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}

		defer rows.Close()

		for rows.Next() {

			err := rows.Scan(&timestampStart, &timestampEnd, &shiftType)
			if err != nil {
				PQErrorHandling(span, sqlStatement, err, false)
				error = err
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
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
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
func GetShifts(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetShifts",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "shiftName"
	data.ColumnNames = []string{"timestamp", JSONColumnName}

	configuration, err := GetCustomerConfiguration(span, customerID)
	if err != nil {
		zap.S().Errorw("GetCustomerConfiguration failed",
			"error", err,
		)
		error = err
		return
	}

	rawShifts, err := GetShiftsRaw(span, customerID, location, asset, from, to, configuration)
	if err != nil {
		zap.S().Errorw("GetShiftsRaw failed",
			"error", err,
		)
		error = err
		return
	}

	processedShifts := cleanRawShiftData(rawShifts, from, to, configuration)
	processedShifts = addNoShiftsBetweenShifts(processedShifts, configuration)

	// Loop through all datapoints
	for _, dataPoint := range processedShifts {
		// TODO: #86 Return timestamps in RFC3339 in /shifts
		fullRow := []interface{}{float64(dataPoint.TimestampBegin.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))), dataPoint.ShiftType}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	return

}

// GetProcessValue gets all data for specific valueName and for a specific asset in a timerange
func GetProcessValue(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time, valueName string) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetProcessValue",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)
	span.SetTag("valueName", valueName)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + valueName

	data.ColumnNames = []string{"timestamp", JSONColumnName}

	sqlStatement := `SELECT timestamp, value FROM processValueTable WHERE asset_id=$1 AND (timestamp BETWEEN $2 AND $3) AND valueName=$4 ORDER BY timestamp ASC;`
	rows, err := db.Query(sqlStatement, assetID, from, to, valueName)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	defer rows.Close()

	for rows.Next() {

		var timestamp time.Time
		var dataPoint float64

		err := rows.Scan(&timestamp, &dataPoint)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}
		fullRow := []interface{}{float64(timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))), dataPoint}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return

}

// GetCurrentState gets the latest state of an asset
func GetCurrentState(parentSpan opentracing.Span, customerID string, location string, asset string, keepStatesInteger bool) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetCurrentState",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("keepStatesInteger", keepStatesInteger)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "state"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	configuration, err := GetCustomerConfiguration(span, customerID)
	if err != nil {
		zap.S().Errorw("GetCustomerConfiguration failed",
			"error", err,
		)
		error = err
		return
	}

	var timestamp time.Time
	var dataPoint int

	sqlStatement := `SELECT timestamp, state FROM stateTable WHERE asset_id=$1 ORDER BY timestamp DESC LIMIT 1;`
	err = db.QueryRow(sqlStatement, assetID).Scan(&timestamp, &dataPoint)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	// Convert old data model to new data model
	dataPoint = datamodel.ConvertOldToNew(dataPoint)

	if keepStatesInteger {
		fullRow := []interface{}{dataPoint, float64(timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
		data.Datapoints = append(data.Datapoints, fullRow)
	} else {
		fullRow := []interface{}{ConvertStateToString(span, dataPoint, 0, configuration), float64(timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
		data.Datapoints = append(data.Datapoints, fullRow)
	}

	return
}

// GetDataTimeRangeForAsset gets the first and latest timestamp. This is used to show all existing data e.g. to create recommendations
func GetDataTimeRangeForAsset(parentSpan opentracing.Span, customerID string, location string, asset string) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetDataTimeRangeForAsset",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}
	data.ColumnNames = []string{"firstTimestamp", "lastTimestamp"}

	var lastTimestampPq pq.NullTime
	var firstTimestampPq pq.NullTime

	var lastTimestamp time.Time
	var firstTimestamp time.Time

	// last timestamp
	sqlStatement := `SELECT MAX(timestamp),MIN(timestamp) FROM stateTable WHERE asset_id=$1;`

	err = db.QueryRow(sqlStatement, assetID).Scan(&lastTimestampPq, &firstTimestampPq)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	if !lastTimestampPq.Valid || !firstTimestampPq.Valid {
		error = errors.New("Asset has no states yet")
		return
	}

	lastTimestamp = lastTimestampPq.Time
	firstTimestamp = firstTimestampPq.Time

	fullRow := []interface{}{firstTimestamp.Format(time.RFC3339), lastTimestamp.Format(time.RFC3339)}
	//fullRow := []float64{float64(firstTimestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))), float64(lastTimestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
	data.Datapoints = append(data.Datapoints, fullRow)

	return
}

// GetCountsRaw gets all states for a specific asset in a timerange
func GetCountsRaw(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time) (data []datamodel.CountEntry, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetCountsRaw",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}

	key := fmt.Sprintf("GetCountsRaw-%d-%s-%s", assetID, from, to)
	if mutex.TryLock(key) { // is is already running?
		defer mutex.Unlock(key)

		// Get from cache if possible
		var cacheHit bool
		data, cacheHit = internal.GetRawCountsFromCache(assetID, from, to)
		if cacheHit { // data found
			zap.S().Debugf("GetCountsRaw cache hit")
			return
		}

		// no data in cache
		sqlStatement := `SELECT timestamp, count, scrap FROM countTable WHERE asset_id=$1 AND timestamp BETWEEN $2 AND $3 ORDER BY timestamp ASC;`
		rows, err := db.Query(sqlStatement, assetID, from, to)
		if err == sql.ErrNoRows {
			PQErrorHandling(span, sqlStatement, err, false)
			return
		} else if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}

		defer rows.Close()

		for rows.Next() {
			var timestamp time.Time
			var dataPoint float64
			var dataPoint2 float64

			err := rows.Scan(&timestamp, &dataPoint, &dataPoint2)
			if err != nil {
				PQErrorHandling(span, sqlStatement, err, false)
				error = err
				return
			}
			fullRow := datamodel.CountEntry{
				Count:     dataPoint,
				Scrap:     dataPoint2,
				Timestamp: timestamp,
			}
			data = append(data, fullRow)
		}
		err = rows.Err()
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}

		internal.StoreRawCountsToCache(assetID, from, to, data)
	} else {
		zap.S().Error("Failed to get Mutex")
	}

	return
}

// GetCounts gets all states for a specific asset in a timerange
func GetCounts(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetCounts",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "count"
	JSONColumnName2 := customerID + "-" + location + "-" + asset + "-" + "scrap"
	data.ColumnNames = []string{JSONColumnName, JSONColumnName2, "timestamp"}

	countSlice, err := GetCountsRaw(span, customerID, location, asset, from, to)
	if err != nil {
		zap.S().Errorf("GetCountsRaw failed", err)
		error = err
		return
	}

	// Loop through all datapoints
	for _, dataPoint := range countSlice {
		fullRow := []interface{}{dataPoint.Count, dataPoint.Scrap, float64(dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
		data.Datapoints = append(data.Datapoints, fullRow)
	}

	return
}

//TODO test GetTotalCounts

// GetTotalCounts gets the sum of produced units for a specific asset in a timerange
func GetTotalCounts(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetTotalCounts",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "count"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	countSlice, err := GetCountsRaw(span, customerID, location, asset, from, to)
	if err != nil {
		zap.S().Errorf("GetCountsRaw failed", err)
		error = err
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
}

// GetProductionSpeed gets the production speed in a selectable interval (in minutes) for a given time range
func GetProductionSpeed(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time, aggregatedInterval int) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetProductionSpeed",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)
	span.SetTag("aggregatedInterval", aggregatedInterval)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "speed"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	//time_bucket_gapfill does not work on Microsoft Azure (license issue)
	sqlStatement := `
	SELECT time_bucket('1 minutes', timestamp) as speedPerIntervall, coalesce(sum(count),0)  
	FROM countTable 
	WHERE asset_id=$1 
		AND timestamp BETWEEN $2 AND $3 
	GROUP BY speedPerIntervall 
	ORDER BY speedPerIntervall ASC;`

	rows, err := db.Query(sqlStatement, assetID, from, to)

	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	defer rows.Close()

	// for custom gapfilling
	var previousTimestamp time.Time

	for rows.Next() {
		var timestamp time.Time
		var dataPoint float64

		err := rows.Scan(&timestamp, &dataPoint)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}

		// TODO: #92 Return timestamps in RFC3339 in /productionSpeed

		// gapfilling to have constant 0 in grafana
		if previousTimestamp.IsZero() != true {
			timeDifference := timestamp.Unix() - previousTimestamp.Unix()

			if timeDifference > 60 { // bigger than one minute
				// add zero speed one minute after previous timestamp
				fullRow := []interface{}{0, float64(previousTimestamp.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond)) + 60*1000)} // 60 = adding 60 seconds
				data.Datapoints = append(data.Datapoints, fullRow)

				// add zero speed one ms before timestamp
				fullRow = []interface{}{0, float64(timestamp.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond)) - 1)} // -1 = subtracting one s
				data.Datapoints = append(data.Datapoints, fullRow)
			}
		}
		// add datapoint
		fullRow := []interface{}{dataPoint * 60, float64(timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))} // *60 to get the production speed per hour
		data.Datapoints = append(data.Datapoints, fullRow)

		previousTimestamp = timestamp
	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return
}

// GetQualityRate gets the quality rate in a selectable interval (in minutes) for a given time range
func GetQualityRate(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time, aggregatedInterval int) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetProductionSpeed",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)
	span.SetTag("aggregatedInterval", aggregatedInterval)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "speed"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	//time_bucket_gapfill does not work on Microsoft Azure (license issue)
	sqlStatement := `
	SELECT time_bucket('1 minutes', timestamp) as ratePerIntervall, 
		coalesce(
			(sum(count)-sum(scrap))::float / sum(count)
		,1)
	FROM countTable 
	WHERE asset_id=$1 
		AND timestamp BETWEEN $2 AND $3 
	GROUP BY ratePerIntervall 
	ORDER BY ratePerIntervall ASC;`

	rows, err := db.Query(sqlStatement, assetID, from, to)

	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	defer rows.Close()

	// for custom gapfilling
	var previousTimestamp time.Time

	for rows.Next() {
		var timestamp time.Time
		var dataPoint float64

		err := rows.Scan(&timestamp, &dataPoint)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}

		// TODO: Return timestamps in RFC3339 in /qualityRate

		// gapfilling to have constant 0 in grafana
		if previousTimestamp.IsZero() != true {
			timeDifference := timestamp.Unix() - previousTimestamp.Unix()

			if timeDifference > 60 { // bigger than one minute
				// add 100% quality one minute after previous timestamp
				fullRow := []interface{}{1, float64(previousTimestamp.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond)) + 60*1000)} // 60 = adding 60 seconds
				data.Datapoints = append(data.Datapoints, fullRow)

				// add 100% one ms before timestamp
				fullRow = []interface{}{1, float64(timestamp.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond)) - 1)} // -1 = subtracting one s
				data.Datapoints = append(data.Datapoints, fullRow)
			}
		}
		// add datapoint
		fullRow := []interface{}{dataPoint, float64(timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
		data.Datapoints = append(data.Datapoints, fullRow)

		previousTimestamp = timestamp
	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return
}

// GetCustomerConfiguration fetches the customer configuration (KPI definition, etc.) from the database
func GetCustomerConfiguration(parentSpan opentracing.Span, customerID string) (configuration datamodel.CustomerConfiguration, error error) {
	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetCustomerConfiguration",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

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
	err := db.QueryRow(sqlStatement, customerID).Scan(
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

	if err == sql.ErrNoRows { // default values if no configuration is stored yet
		configuration.MicrostopDurationInSeconds = 60 * 2
		configuration.IgnoreMicrostopUnderThisDurationInSeconds = -1 // do not apply
		configuration.MinimumRunningTimeInSeconds = 0
		configuration.ThresholdForNoShiftsConsideredBreakInSeconds = 60 * 35
		configuration.LowSpeedThresholdInPcsPerHour = -1 // do not apply by default
		configuration.AutomaticallyIdentifyChangeovers = true
		configuration.AvailabilityLossStates = append(configuration.AvailabilityLossStates, 40000, 180000, 190000, 200000, 210000, 220000)
		configuration.PerformanceLossStates = append(configuration.PerformanceLossStates, 20000, 50000, 60000, 70000, 80000, 90000, 100000, 110000, 120000, 130000, 140000, 150000)
		configuration.LanguageCode = 0
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	configuration.AvailabilityLossStates = []int32(tempAvailabilityLossStates)
	configuration.PerformanceLossStates = []int32(tempPerformanceLossStates)

	// Store to cache if not yet existing
	go internal.StoreCustomerConfigurationToCache(customerID, configuration)
	zap.S().Debug("Stored configuration to cache")

	return
}

// GetRecommendations gets all current recommendations for a specific asset
func GetRecommendations(parentSpan opentracing.Span, customerID string, location string, asset string) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetRecommendations",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)

	data.ColumnNames = []string{"timestamp", "recommendationType", "recommendationValues", "recommendationTextEN", "recommendationTextDE", "diagnoseTextEN", "diagnoseTextDE"}

	var likeString string = customerID + "-" + location + "-" + asset + "-%"
	sqlStatement := `
	SELECT timestamp, recommendationType, recommendationValues, recommendationTextEN, recommendationTextDE, diagnoseTextEN, diagnoseTextDE 
	FROM recommendationTable 
	WHERE enabled=True 
		AND uid LIKE $1 
		AND (timestamp = (SELECT MAX(timestamp) FROM recommendationTable WHERE enabled=True AND uid LIKE $1)) 
	ORDER BY timestamp DESC;`
	// AND (timestamp=) used to only get the recommendations from the latest calculation batch (avoid showing old ones)
	rows, err := db.Query(sqlStatement, likeString)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
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

		err := rows.Scan(&timestamp, &recommendationType, &recommendationValues, &recommendationTextEN, &recommendationTextDE, &diagnoseTextEN, &diagnoseTextDE)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}

		// TODO: #87 Return timestamps in RFC3339 in /recommendations
		fullRow := []interface{}{float64(timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))), recommendationType, recommendationValues, recommendationTextEN, recommendationTextDE, diagnoseTextEN, diagnoseTextDE}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return
}

// GetMaintenanceActivities gets all maintenance activities for a specific asset
func GetMaintenanceActivities(parentSpan opentracing.Span, customerID string, location string, asset string) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetMaintenanceActivities",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}
	data.ColumnNames = []string{"Component", "Activity", "Timestamp"}

	sqlStatement := `
	SELECT componentname, activitytype, timestamp 
	FROM maintenanceActivities 
	INNER JOIN componentTable ON 
		(maintenanceActivities.component_id = componentTable.id) 
	WHERE component_id IN (SELECT component_id FROM componentTable WHERE asset_id = $1);`

	rows, err := db.Query(sqlStatement, assetID)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	defer rows.Close()

	for rows.Next() {

		var componentName string
		var activityType int
		var timestamp time.Time

		err := rows.Scan(&componentName, &activityType, &timestamp)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}
		fullRow := []interface{}{componentName, activityType, float64(timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return
}

// GetUniqueProducts gets all unique products for a specific asset in a specific time range
func GetUniqueProducts(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetUniqueProducts",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}
	data.ColumnNames = []string{"UID", "Timestamp begin", "Timestamp end", "Product ID", "Is Scrap", "Quality class", "Station ID"}

	sqlStatement := `
	SELECT uid, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap, quality_class, station_id 
	FROM uniqueProductTable 
	WHERE asset_id = $1 
		AND (begin_timestamp_ms BETWEEN $2 AND $3 OR end_timestamp_ms BETWEEN $2 AND $3) 
		OR (begin_timestamp_ms < $2 AND end_timestamp_ms > $3) 
	ORDER BY begin_timestamp_ms ASC;`

	rows, err := db.Query(sqlStatement, assetID, from, to)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	defer rows.Close()

	for rows.Next() {

		var UID string
		var timestampBegin time.Time
		var timestampEnd time.Time
		var productID string
		var isScrap bool
		var qualityClass string
		var stationID string

		err := rows.Scan(&UID, &timestampBegin, &timestampEnd, &productID, &isScrap, &qualityClass, &stationID)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}
		fullRow := []interface{}{
			UID,
			float64(timestampBegin.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))),
			float64(timestampEnd.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))),
			productID,
			isScrap,
			qualityClass,
			stationID,
		}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return
}

// GetUpcomingTimeBasedMaintenanceActivities returns UpcomingTimeBasedMaintenanceActivities array for an asset
func GetUpcomingTimeBasedMaintenanceActivities(parentSpan opentracing.Span, customerID string, location string, asset string) (data []datamodel.UpcomingTimeBasedMaintenanceActivities, error error) {
	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetUpcomingTimeBasedMaintenanceActivities",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
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

	rows, err := db.Query(sqlStatement, assetID)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
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

		err := rows.Scan(&ComponentName, &IntervallInHours, &ActivityType, &LatestActivity, &NextActivity, &Duration)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
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
func GetOrdersRaw(parentSpan opentracing.Span, customerID string, location string, asset string, from time.Time, to time.Time) (data []datamodel.OrdersRaw, error error) {
	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetOrdersRaw",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
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

	rows, err := db.Query(sqlStatement, assetID, from, to)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
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

		err := rows.Scan(&orderName, &targetUnits, &beginTimestamp, &endTimestamp, &productName, &timePerUnitInSeconds)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
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

// GetDistinctProcessValues gets all possible process values for a specific asset. It returns an array of strings with every string starting with process_
func GetDistinctProcessValues(parentSpan opentracing.Span, customerID string, location string, asset string) (data []string, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetDistinctProcessValues",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}

	sqlStatement := "SELECT distinct valueName FROM processValueTable WHERE asset_id=$1;"
	rows, err := db.Query(sqlStatement, assetID)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	defer rows.Close()

	for rows.Next() {
		var currentString string

		err := rows.Scan(&currentString)
		if err != nil {
			PQErrorHandling(span, sqlStatement, err, false)
			error = err
			return
		}

		data = append(data, "process_"+currentString)
	}

	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	return
}

// GetAssetID gets the assetID from the database
func GetAssetID(parentSpan opentracing.Span, customerID string, location string, assetID string) (DBassetID int, error error) {
	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetAssetID",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	// Get from cache if possible
	var cacheHit bool
	DBassetID, cacheHit = internal.GetAssetIDFromCache(customerID, location, assetID)
	if cacheHit { // data found
		// zap.S().Debugf("GetAssetID cache hit")
		return
	}

	sqlStatement := "SELECT id FROM assetTable WHERE assetid=$1 AND location=$2 AND customer=$3;"
	err := db.QueryRow(sqlStatement, assetID, location, customerID).Scan(&DBassetID)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatement, err, false)
		error = errors.New("Asset does not exist")
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatement, err, false)
		error = err
		return
	}

	// Store to cache if not yet existing
	go internal.StoreAssetIDToCache(customerID, location, assetID, DBassetID)
	zap.S().Debug("Stored AssetID to cache")

	return
}

// GetUniqueProductsWithTags gets all unique products with tags and parents for a specific asset in a specific time range
func GetUniqueProductsWithTags(parentSpan opentracing.Span, customerID string, location string, asset string,
	from time.Time, to time.Time) (data datamodel.DataResponseAny, error error) {

	// Jaeger tracing
	span := opentracing.StartSpan(
		"GetUniqueProductsWithTags",
		opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	span.SetTag("customerID", customerID)
	span.SetTag("location", location)
	span.SetTag("asset", asset)
	span.SetTag("from", from)
	span.SetTag("to", to)

	assetID, err := GetAssetID(span, customerID, location, asset)
	if err != nil {
		error = err
		return
	}

	//getting the data
	sqlStatementData := `
	SELECT uniqueProductID, uniqueProductAlternativeID, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap, valueName, value
	FROM uniqueProductTable 
		LEFT JOIN productTagTable ON uniqueProductTable.uniqueProductID = productTagTable.product_uid
	WHERE asset_id = $1 
		AND (begin_timestamp_ms BETWEEN $2 AND $3 OR end_timestamp_ms BETWEEN $2 AND $3) 
		OR (begin_timestamp_ms < $2 AND end_timestamp_ms > $3) 
	ORDER BY uniqueProductID ASC;` // fix name for product table in <productTagTable.product_uid>
	// use 2 sql statements 1 for tags with scalar values 1 for tags with string values
	// see http://go-database-sql.org/retrieving.html
	sqlStatementDataStrings := `
	SELECT uniqueProductID, uniqueProductAlternativeID, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap, valueName, value
	FROM uniqueProductTable 
		INNER JOIN productTagStringTable ON uniqueProductTable.uniqueProductID = productTagStringTable.product_uid
	WHERE asset_id = $1 
		AND (begin_timestamp_ms BETWEEN $2 AND $3 OR end_timestamp_ms BETWEEN $2 AND $3) 
		OR (begin_timestamp_ms < $2 AND end_timestamp_ms > $3) 
	ORDER BY uniqueProductID ASC;`

	rows, err := db.Query(sqlStatementData, assetID, from, to)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatementData, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatementData, err, false)
		error = err
		return
	}

	defer rows.Close()

	rowsStrings, err := db.Query(sqlStatementDataStrings, assetID, from, to)
	if err == sql.ErrNoRows {
		PQErrorHandling(span, sqlStatementDataStrings, err, false)
		return
	} else if err != nil {
		PQErrorHandling(span, sqlStatementDataStrings, err, false)
		error = err
		return
	}

	defer rowsStrings.Close()

	//Defining the base column names
	data.ColumnNames = []string{"UID", "TimestampBegin", "TimestampEnd", "ProductID", "IsScrap"}
	var indexRow int
	var indexColumn int

	//Rows can contain valueName and value or not: if not, they contain null
	for rows.Next() {

		var UID int
		var AID string
		var timestampBegin time.Time
		var timestampEnd sql.NullTime
		var productID int
		var isScrap bool
		var valueName sql.NullString
		var value sql.NullFloat64

		err := rows.Scan(&UID, &AID, &timestampBegin, &timestampEnd, &productID, &isScrap, &valueName, &value)
		if err != nil {
			PQErrorHandling(span, sqlStatementData, err, false)
			error = err
			return
		}

		//if productTag name not in data.ColumnNames yet, add to data.ColumnNames, store index of column for data.DataPoints and extend slice
		if valueName.Valid {
			data.Datapoints, data.ColumnNames, indexColumn = ChangeOutputFormat(data.Datapoints, data.ColumnNames, valueName.String)
		}

		if data.Datapoints == nil {		//if no row in data.Datapoints, create new row
			var fullRow []interface{}
			fullRow = append(fullRow, UID)
			fullRow = append(fullRow, AID)
			fullRow = append(fullRow, float64(timestampBegin.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))))
			if timestampEnd.Valid {
				fullRow = append(fullRow, float64(timestampEnd.Time.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))))
			} else {
				fullRow = append(fullRow, nil)
			}
			fullRow = append(fullRow, productID)
			fullRow = append(fullRow, isScrap)
			fullRow =  LengthenSliceToFitNames(fullRow, data.ColumnNames)
			if valueName.Valid == true && value.Valid == true { //if a value is specified, add to data.Datapoints
				fullRow[indexColumn] = value.Float64
			}
			data.Datapoints = append(data.Datapoints, fullRow)
		} else { //if there are already rows in Data.datapoint
			//if same uid as row before, add value to datapoint
			indexRow = len(data.Datapoints) - 1
			lastUID, ok := data.Datapoints[indexRow][0].(int)
			if ok == false {
				zap.S().Errorf("GetUniqueProductsWithTags: casting lastUID to int error", UID, timestampBegin)
				return
			}
			if UID == lastUID && value.Valid && valueName.Valid {
				data.Datapoints[indexRow][indexColumn] = value.Float64
			} else if UID == lastUID && (!value.Valid || !valueName.Valid){ //if there are multiple lines with the same UID, each line should have a correct productTag
				zap.S().Errorf("GetUniqueProductsWithTags: value.Valid or valueName.Valid false where it shouldn't", UID, timestampBegin)
				return
			} else if UID != lastUID { //create new row in tempDataPoints
				var fullRow []interface{}
				fullRow = append(fullRow, UID)
				fullRow = append(fullRow, AID)
				fullRow = append(fullRow, float64(timestampBegin.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))))
				if timestampEnd.Valid {
					fullRow = append(fullRow, float64(timestampEnd.Time.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))))
				} else {
					fullRow = append(fullRow, nil)
				}
				fullRow = append(fullRow, productID)
				fullRow = append(fullRow, isScrap)

				fullRow =  LengthenSliceToFitNames(fullRow, data.ColumnNames)
				if valueName.Valid == true && value.Valid == true { //if a value is specified, add to data.Datapoints
					fullRow[indexColumn] = value.Float64
				}
				data.Datapoints = append(data.Datapoints, fullRow)
			} else {
				zap.S().Errorf("GetUniqueProductsWithTags: logic error", UID, timestampBegin)
				return
			}
		}

	}
	err = rows.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatementData, err, false)
		error = err
		return
	}

	// uid, valueName and value should always exist
	for rowsStrings.Next() {
		var UID int
		var AID string
		var timestampBegin time.Time
		var timestampEnd sql.NullTime
		var productID int
		var isScrap bool
		var valueName sql.NullString
		var value sql.NullString

		err := rowsStrings.Scan(&UID, &AID, &timestampBegin, &timestampEnd, &productID, &isScrap, &valueName, &value)
		if err != nil {
			PQErrorHandling(span, sqlStatementData, err, false)
			error = err
			return
		}
		if !valueName.Valid || !value.Valid{
			zap.S().Errorf("GetUniqueProductsWithTags: valueName or value for productTagString not valid", UID, timestampBegin)
			return
		}
		//if productTagString name not yet known, add to data.ColumnNames, store index for data.DataPoints in newColumns and extend slice
		data.Datapoints, data.ColumnNames, indexColumn = ChangeOutputFormat(data.Datapoints, data.ColumnNames, valueName.String)
		var contains bool
		contains, indexRow = SliceContainsInt(data.Datapoints, UID, 0)

		if contains { //true if uid already in data.Datapoints
			data.Datapoints[indexRow][indexColumn] = value.String
		} else { //throw error
			zap.S().Errorf("GetUniqueProductsWithTags: UID not found: Error!", UID, timestampBegin)
			return
		}
	}
	err = rowsStrings.Err()
	if err != nil {
		PQErrorHandling(span, sqlStatementDataStrings, err, false)
		error = err
		return
	}
	return
}


