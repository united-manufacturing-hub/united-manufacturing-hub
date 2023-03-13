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

/*


import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"os/signal"
	"syscall"
	"time"
)

var (
	db                      = database.Db
	logData                 = false
	Mutex                   = database.Mutex
	GracefulShutdownChannel = database.GracefulShutdownChannel
)

func GetEnterpriseId(enterpriseName string) (enterpriseId uint32, err error) {
	zap.S().Infof("[GetEnterpriseID] enterpriseName: %v", enterpriseName)

	// Get from cache if possible
	var cacheHit bool
	enterpriseId, cacheHit = internal.GetEnterpriseIDFromCache(enterpriseName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM enterpriseTable WHERE name = $1`

	err = db.QueryRow(sqlStatement, enterpriseName).Scan(&enterpriseId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetEnterpriseID] enterprise not found: %v", enterpriseName)
		err = errors.New("enterprise not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreEnterpriseIDToCache(enterpriseName, enterpriseId)
	return
}

func GetSiteId(enterpriseId uint32, siteName string) (siteId uint32, err error) {
	zap.S().Infof("[GetSiteID] enterpriseId: %v, siteName: %v", enterpriseId, siteName)

	// Get from cache if possible
	var cacheHit bool
	siteId, cacheHit = internal.GetSiteIDFromCache(enterpriseId, siteName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM siteTable WHERE enterpriseId = $1 AND name = $2`

	err = db.QueryRow(sqlStatement, enterpriseId, siteName).Scan(&siteId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetSiteID] site not found: %v", siteName)
		err = errors.New("site not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreSiteIDToCache(enterpriseId, siteName, siteId)
	return
}

func GetAreaId(siteId uint32, areaName string) (areaId uint32, err error) {
	zap.S().Infof("[GetAreaID] siteId: %v, areaName: %v", siteId, areaName)

	// Get from cache if possible
	var cacheHit bool
	areaId, cacheHit = internal.GetAreaIDFromCache(siteId, areaName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM areaTable WHERE siteId = $1 AND name = $2`

	err = db.QueryRow(sqlStatement, siteId, areaName).Scan(&areaId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetAreaID] area not found: %v", areaName)
		err = errors.New("area not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreAreaIDToCache(siteId, areaName, areaId)
	return
}

func GetProductionLineId(areaId uint32, productionLineName string) (productionLineId uint32, err error) {
	zap.S().Infof("[GetProductionLineID] areaId: %v, productionLineName: %v", areaId, productionLineName)

	// Get from cache if possible
	var cacheHit bool
	productionLineId, cacheHit = internal.GetProductionLineIDFromCache(areaId, productionLineName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM productionLineTable WHERE areaId = $1 AND name = $2`

	err = db.QueryRow(sqlStatement, areaId, productionLineName).Scan(&productionLineId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetProductionLineID] production line not found: %v", productionLineName)
		err = errors.New("production line not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreProductionLineIDToCache(areaId, productionLineName, productionLineId)
	return
}

func GetWorkCellId(productionLineId uint32, workCellName string) (workCellId uint32, err error) {
	zap.S().Infof("[GetWorkCellID] productionLineId: %v, workCellName: %v", productionLineId, workCellName)

	// Get from cache if possible
	var cacheHit bool
	workCellId, cacheHit = internal.GetWorkCellIDFromCache(productionLineId, workCellName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM workCellTable WHERE productionLineId = $1 AND name = $2`

	err = db.QueryRow(sqlStatement, productionLineId, workCellName).Scan(&workCellId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetWorkCellID] work cell not found: %v", workCellName)
		err = errors.New("work cell not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreWorkCellIDToCache(productionLineId, workCellName, workCellId)
	return
}

// GetEnterpriseConfiguration fetches the enterprise configuration (KPI definition, etc.) from the database
func GetEnterpriseConfiguration(enterpriseName string) (configuration datamodel.EnterpriseConfiguration, err error) {
	zap.S().Infof("[GetEnterpriseConfiguration] enterpriseName: %v", enterpriseName)

	// Get from cache if possible
	var cacheHit bool
	configuration, cacheHit = internal.GetEnterpriseConfigurationFromCache(enterpriseName)
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
	err = db.QueryRow(sqlStatement, enterpriseName).Scan(
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
		zap.S().Warnf("No configuration stored for enterprise %s, using default !", enterpriseName)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	configuration.AvailabilityLossStates = tempAvailabilityLossStates
	configuration.PerformanceLossStates = tempPerformanceLossStates

	// Store to cache if not yet existing
	go internal.StoreEnterpriseConfigurationToCache(enterpriseName, configuration)
	zap.S().Debug("Stored configuration to cache")

	return
}

// GetStatesRaw gets all states for a specific work cell in a timerange. It returns an array of datamodel.StateEntry
func GetStatesRaw(
	workCellId uint32,
	from, to time.Time,
	configuration datamodel.EnterpriseConfiguration) (data []datamodel.StateEntry, err error) {
	zap.S().Infof(
		"[GetStatesRaw] workCellId: %v, from: %v, to: %v, configuration: %v",
		workCellId,
		from,
		to,
		configuration)

	key := fmt.Sprintf("GetStatesRaw-%d-%s-%s-%s", workCellId, from, to, internal.AsHash(configuration))
	if Mutex.TryLock(key) { // is is already running?
		defer Mutex.Unlock(key)

		// Get from cache if possible
		var cacheHit bool
		customerConfiguration := configuration.ConvertEnterpriseToCustomerConfiguration()
		// TODO: GetStatesRawFromCache for EnterpriseConfiguration (V2 maybe? Or a cache service inside v3/services/ with the caching function specific to v3?)
		data, cacheHit = internal.GetStatesRawFromCache(workCellId, from, to, customerConfiguration)
		if cacheHit { // data found
			zap.S().Debugf("GetStatesRaw cache hit")
			return
		}

		// no data in cache

		// Additionally, get the latest state before the time range
		var timestamp time.Time
		var dataPoint int

		sqlStatement := `SELECT timestamp, state FROM stateTable WHERE workCellId=$1 AND timestamp < $2 ORDER BY timestamp DESC LIMIT 1;`

		err = db.QueryRow(sqlStatement, workCellId, from).Scan(&timestamp, &dataPoint)
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

		sqlStatement = `SELECT timestamp, state FROM stateTable WHERE workCellId=$1 AND timestamp BETWEEN $2 AND $3 ORDER BY timestamp ASC;`

		var rows *sql.Rows
		rows, err = db.Query(sqlStatement, workCellId, from, to)
		if errors.Is(err, sql.ErrNoRows) {
			database.ErrorHandling(sqlStatement, err, false)
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

		internal.StoreRawStatesToCache(workCellId, from, to, customerConfiguration, data)

	} else {
		zap.S().Error("Failed to get Mutex")
	}

	return
}

// GetShiftsRaw gets all shifts for a specific work cell in a timerange in a raw format
func GetShiftsRaw(
	workCellId uint32,
	from, to time.Time,
	configuration datamodel.EnterpriseConfiguration) (data []datamodel.ShiftEntry, err error) {
	zap.S().Infof(
		"[GetShiftsRaw] workCellId: %v, from: %v, to: %v, configuration: %v",
		workCellId,
		from,
		to,
		configuration)

	key := fmt.Sprintf("GetShiftsRaw-%d-%s-%s-%s", workCellId, from, to, internal.AsHash(configuration))
	if Mutex.TryLock(key) { // is is already running?
		defer Mutex.Unlock(key)

		// Get from cache if possible
		var cacheHit bool
		customerConfiguration := configuration.ConvertEnterpriseToCustomerConfiguration()
		// TODO: GetStatesRawFromCache for EnterpriseConfiguration (V2 maybe? Or a cache service inside v3/services/ with the caching function specific to v3?)
		data, cacheHit = internal.GetRawShiftsFromCache(workCellId, from, to, customerConfiguration)
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
		WHERE workCellId=$1
			AND ((begin_timestamp BETWEEN $2 AND $3 OR end_timestamp BETWEEN $2 AND $3)
			OR (begin_timestamp < $2 AND end_timestamp > $3))
		ORDER BY begin_timestamp ASC LIMIT 1;
		`

		err = db.QueryRow(sqlStatement, workCellId, from, to).Scan(&timestampStart, &timestampEnd, &shiftType)
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
		WHERE workCellId=$1
			AND ((begin_timestamp BETWEEN $2 AND $3 OR end_timestamp BETWEEN $2 AND $3)
			OR (begin_timestamp < $2 AND end_timestamp > $3))
		ORDER BY begin_timestamp ASC OFFSET 1;`

		var rows *sql.Rows
		rows, err = db.Query(sqlStatement, workCellId, from, to) // OFFSET to prevent entering first result twice
		if errors.Is(err, sql.ErrNoRows) {
			database.ErrorHandling(sqlStatement, err, false)
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

		internal.StoreRawShiftsToCache(workCellId, from, to, customerConfiguration, data)
	} else {
		zap.S().Error("Failed to get Mutex")
	}

	return
}

// GetShifts gets all shifts for a specific asset in a timerange
func GetShifts(enterpriseName string, workCellId uint32, from, to time.Time) (
	data datamodel.DataResponseAny,
	err error) {
	zap.S().Infof("[GetShiftsRaw] workCellId: %v, from: %v, to: %v", workCellId, from, to)

	// TODO: adapt JSONColumnName to new data model
	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "shiftName"
	data.ColumnNames = []string{"timestamp", JSONColumnName}

	var configuration datamodel.EnterpriseConfiguration
	configuration, err = GetEnterpriseConfiguration(enterpriseName)
	if err != nil {
		zap.S().Errorw(
			"GetEnterpriseConfiguration failed",
			"error", err,
		)
		return
	}

	var rawShifts []datamodel.ShiftEntry
	rawShifts, err = GetShiftsRaw(workCellId, from, to, configuration)
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
		fullRow := []interface{}{
			float64(dataPoint.TimestampBegin.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))),
			dataPoint.ShiftType}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	return

}

// GetCountsRaw gets all states for a specific work cell in a timerange
func GetCountsRaw(workCellId uint32, from, to time.Time) (data []datamodel.CountEntry, err error) {
	zap.S().Infof("[GetCountsRaw] workCellId: %v from: %v, to: %v", workCellId, from, to)

	key := fmt.Sprintf("GetCountsRaw-%d-%s-%s", workCellId, from, to)
	if Mutex.TryLock(key) { // is is already running?
		defer Mutex.Unlock(key)

		// Get from cache if possible
		var cacheHit bool
		data, cacheHit = internal.GetRawCountsFromCache(workCellId, from, to)
		if cacheHit { // data found
			zap.S().Debugf("GetCountsRaw cache hit")
			return
		}

		// no data in cache
		// TODO: update query and implementation, no "scrap" field in new datamodel
		sqlStatement := `SELECT timestamp, count, scrap FROM countTable WHERE workCellId=$1 AND timestamp BETWEEN $2 AND $3 ORDER BY timestamp ASC;`
		var rows *sql.Rows
		rows, err = db.Query(sqlStatement, workCellId, from, to)
		if errors.Is(err, sql.ErrNoRows) {
			database.ErrorHandling(sqlStatement, err, false)
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

		internal.StoreRawCountsToCache(workCellId, from, to, data)
	} else {
		zap.S().Error("Failed to get Mutex")
	}

	return
}

// GetCounts gets all states for a specific asset in a timerange
func GetCounts(workCellId uint32, from, to time.Time) (data datamodel.DataResponseAny, err error) {
	zap.S().Infof("[GetCounts] workCellId: %v from: %v, to: %v", workCellId, from, to)

	// TODO: adapt JSONColumnName to new data model
	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "count"
	JSONColumnName2 := customerID + "-" + location + "-" + asset + "-" + "scrap"
	data.ColumnNames = []string{JSONColumnName, JSONColumnName2, "timestamp"}

	var countSlice []datamodel.CountEntry
	countSlice, err = GetCountsRaw(workCellId, from, to)
	if err != nil {
		zap.S().Errorf("GetCountsRaw failed", err)

		return
	}

	// Loop through all datapoints
	for _, dataPoint := range countSlice {
		fullRow := []interface{}{
			dataPoint.Count,
			dataPoint.Scrap,
			float64(dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
		data.Datapoints = append(data.Datapoints, fullRow)
	}

	return
}

// GetOrdersRaw gets all order and product infirmation in a specific time range for a work cell
func GetOrdersRaw(workCellId uint32, from, to time.Time) (data []datamodel.OrdersRaw, err error) {
	zap.S().Infof("[GetOrdersRaw] workCellId: %v from: %v, to: %v", workCellId, from, to)

	sqlStatement := `
		SELECT order_name, target_units, begin_timestamp, end_timestamp, productTable.product_name, productTable.time_per_unit_in_seconds
		FROM orderTable
		FULL JOIN productTable ON productTable.product_id = orderTable.product_id
		WHERE
			begin_timestamp IS NOT NULL
			AND end_timestamp IS NOT NULL
			AND orderTable.workCellId = $1
			AND (
				(begin_timestamp BETWEEN $2 AND $3 OR end_timestamp BETWEEN $2 AND $3)
				OR (begin_timestamp < $2 AND end_timestamp > $3)
			)
		ORDER BY begin_timestamp ASC;
	`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, workCellId, from, to)
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

// GetOrdersTimeline gets all orders for a specific asset in a timerange for a timeline
func GetOrdersTimeline(workCellId uint32, from, to time.Time) (data datamodel.DataResponseAny, err error) {

	// TODO: adapt JSONColumnName to new data model

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "order"
	data.ColumnNames = []string{"timestamp", JSONColumnName}

	// configuration := getCustomerConfiguration(span, customerID, location, asset)

	rawOrders, err := GetOrdersRaw(workCellId, from, to)
	if err != nil {
		zap.S().Errorf("GetOrdersRaw failed", err)

		return
	}

	processedOrders := addNoOrdersBetweenOrders(rawOrders, from, to)

	// Loop through all datapoints
	for _, dataPoint := range processedOrders {
		fullRow := []interface{}{
			float64(dataPoint.TimestampBegin.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))),
			dataPoint.OrderType}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	return
	* /
}

// GetProductionSpeed gets the production speed in a selectable interval (in minutes) for a given time range
func GetProductionSpeed(workCellId uint32, from, to time.Time, aggregatedInterval int) (
	data datamodel.DataResponseAny,
	err error) {
	// TODO: adapt JSONColumnName to new data model

	zap.S().Infof(
		"[GetProductionSpeed] workCellId: %v from: %v, to: %v, aggregatedInterval: %v",
		workCellId,
		from,
		to,
		aggregatedInterval)

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "speed"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// time_bucket_gapfill does not work on Microsoft Azure (license issue)
	sqlStatement := `
		SELECT time_bucket('1 minutes', timestamp) as speedPerIntervall, coalesce(sum(count),0)
		FROM countTable
		WHERE workCellId=$1
			AND timestamp BETWEEN $2 AND $3
		GROUP BY speedPerIntervall
		ORDER BY speedPerIntervall ASC;`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, workCellId, from, to)

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
				// add zero speed one minute after previous timestamp
				fullRow := []interface{}{
					0,
					float64(previousTimestamp.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond)) + 60*1000)} // 60 = adding 60 seconds
				data.Datapoints = append(data.Datapoints, fullRow)

				// add zero speed one ms before timestamp
				fullRow = []interface{}{
					0,
					float64(timestamp.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond)) - 1)} // -1 = subtracting one s
				data.Datapoints = append(data.Datapoints, fullRow)
			}
		}
		// add datapoint
		fullRow := []interface{}{
			dataPoint * 60,
			float64(timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))} // *60 to get the production speed per hour
		data.Datapoints = append(data.Datapoints, fullRow)

		previousTimestamp = timestamp
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return

	* /
}

// BusinessLogicErrorHandling logs and handles errors during the business logic
func BusinessLogicErrorHandling(operationName string, err error, isCritical bool) {

	zap.S().Errorw(
		"Error in business logic. ",
		"operation name", operationName,
		"error", err.Error(),
	)
	if isCritical {
		signal.Notify(GracefulShutdownChannel, syscall.SIGTERM)
	}
}
*/
