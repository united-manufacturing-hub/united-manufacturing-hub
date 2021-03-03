package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/omeid/pgerror"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

var db *sql.DB

var isDryRun bool

type processValue64 struct {
	TimestampMs int64
	DBassetID   int
	Value       float64
	ValueName   string
}

type countStruct struct {
	TimestampMs int64
	DBassetID   int
	Count       int
	Scrap       int
}

var processValue64Channel chan processValue64
var countStructChannel chan countStruct

// SetupDB setups the db and stores the handler in a global variable in database.go
func SetupDB(PQUser string, PQPassword string, PWDBName string, PQHost string, PQPort int, health healthcheck.Handler, dryRun string) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=require", PQHost, PQPort, PQUser, PQPassword, PWDBName)
	var err error
	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		PQErrorHandling("sql.Open()", err)
	}

	if dryRun == "True" || dryRun == "true" {
		zap.S().Infof("Running in DRY_RUN mode. ALl statements will be rolled back and printed automatically")
		isDryRun = true
	} else {
		isDryRun = false
	}

	// Healthcheck
	health.AddReadinessCheck("database", healthcheck.DatabasePingCheck(db, 1*time.Second))

	processValue64Channel = make(chan processValue64, 1000) //Buffer size of 1000
	countStructChannel = make(chan countStruct, 1000)       //Buffer size of 1000

	go storeProcessValue64BufferIntoDatabase()
	go storeCountBufferIntoDatabase()
}

// ShutdownDB closes all database connections
func ShutdownDB() {
	err := db.Close()
	if err != nil {
		panic(err)
	}
}

// PQErrorHandling logs and handles postgresql errors
func PQErrorHandling(sqlStatement string, err error) {

	if e := pgerror.UniqueViolation(err); e != nil {
		zap.S().Warnf("PostgreSQL failed: UniqueViolation", err, sqlStatement)
		return
	} else if e := pgerror.CheckViolation(err); e != nil {
		zap.S().Warnf("PostgreSQL failed: CheckViolation", err, sqlStatement)
		return
	}

	zap.S().Errorf("PostgreSQL failed.", err, sqlStatement)
	ShutdownApplicationGraceful()
}

func storeIntoTable(timestampMs int64, DBassetID int, tableName string, value int, columnName string) {
	zap.S().Debugf("storeIntoTable called", timestampMs, DBassetID, columnName, value)

	//https://stackoverflow.com/questions/23950025/how-to-write-bigint-timestamp-in-milliseconds-value-as-timestamp-in-postgresql

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	// WARNING SQL INJECTION POSSIBLE
	sqlStatement := `
		INSERT INTO ` + tableName + `(timestamp, asset_id, ` + columnName + `) 
		VALUES (to_timestamp($1 / 1000.0),$2,$3) 
		ON CONFLICT DO NOTHING;`

	_, err = tx.ExecContext(ctx, sqlStatement, timestampMs, DBassetID, value)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// StoreIntoprocessValueTable stores a state into the database
func StoreIntoprocessValueTable(timestampMs int64, DBassetID int, value int, valueName string) {
	zap.S().Debugf("StoreIntoprocessValueTable called", timestampMs, DBassetID, value, valueName)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		INSERT INTO processValueTable (timestamp, asset_id, value, valuename) 
		VALUES (to_timestamp($1 / 1000.0),$2,$3,$4) 
		ON CONFLICT DO NOTHING;`

	_, err = tx.ExecContext(ctx, sqlStatement, timestampMs, DBassetID, value, valueName)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// StoreRecommendation stores a recommendation into the database
func StoreRecommendation(timestampMs int64, uid string, recommendationType int, enabled bool, recommendationValues string, recommendationTextEN string, recommendationTextDE string, recommendationDiagnoseTextDE string, recommendationDiagnoseTextEN string) {
	zap.S().Debugf("StoreRecommendation called", timestampMs, uid, recommendationType, enabled, recommendationValues)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		INSERT INTO recommendationTable (timestamp, uid, recommendationType, enabled, recommendationValues, recommendationTextEN, recommendationTextDE, diagnoseTextEN, diagnoseTextDE) 
		VALUES (to_timestamp($1 / 1000.0),$2,$3,$4,$5,$6,$7,$8,$9) 
		ON CONFLICT (uid) DO UPDATE 
		SET timestamp=to_timestamp($1 / 1000.0), uid=$2, recommendationType=$3, enabled=$4, recommendationValues=$5, recommendationTextEN=$6, recommendationTextDE=$7, diagnoseTextEN=$8, diagnoseTextDE=$9;`

	_, err = tx.ExecContext(ctx, sqlStatement, timestampMs, uid, recommendationType, enabled, recommendationValues, recommendationTextEN, recommendationTextDE, recommendationDiagnoseTextEN, recommendationDiagnoseTextDE)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// StoreIntoprocessValueTableFloat64 stores a state into the database
func StoreIntoprocessValueTableFloat64(timestampMs int64, DBassetID int, value float64, valueName string) {
	zap.S().Debugf("StoreIntoprocessValueTableFloat64 called", timestampMs, DBassetID, value, valueName)
	processValue := processValue64{
		TimestampMs: timestampMs,
		DBassetID:   DBassetID,
		Value:       value,
		ValueName:   valueName,
	}

	processValue64Channel <- processValue

	/*
		sqlStatement := `
			INSERT INTO processValueTable (timestamp, asset_id, value, valuename)
			VALUES (to_timestamp($1 / 1000.0),$2,$3,$4)
			ON CONFLICT DO NOTHING;`

		_, err := db.Exec(sqlStatement, timestampMs, DBassetID, value, valueName)
		if err != nil {
			zap.S().Errorf("INSERT failed", err, sqlStatement)
			panic(err)
		}
	*/
}

func getProcessValue64BufferAndStore() {
	txn, err := db.Begin()
	if err != nil {
		PQErrorHandling("db.Begin()", err)
	}

	stmt, err := txn.Prepare(pq.CopyIn("processvaluetable", "timestamp", "asset_id", "value", "valuename"))
	if err != nil {
		PQErrorHandling("pq.CopyIn()", err)
	}

	keepRunning := false
	for !keepRunning {
		select {
		case pt := <-processValue64Channel:

			timestamp := time.Unix(0, pt.TimestampMs*int64(1000000)).Format("2006-01-02T15:04:05.000Z")
			zap.S().Debugf("getProcessValue64BufferAndStore called", pt.TimestampMs, timestamp, pt.DBassetID, pt.Value, pt.ValueName)
			_, err = stmt.Exec(timestamp, pt.DBassetID, pt.Value, pt.ValueName)
			if err != nil {
				PQErrorHandling("stmt.Exec()", err)
			}
		default:
			keepRunning = true
			break
		}
	}

	_, err = stmt.Exec()
	if err != nil {
		PQErrorHandling("stmt.Exec()", err)
	}

	err = stmt.Close()
	if err != nil {
		PQErrorHandling("stmt.Close()", err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", "getProcessValue64BufferAndStore")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		return
	}

	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
	}

}

func storeProcessValue64BufferIntoDatabase() { //Send the buffer every second into database
	for range time.Tick(time.Duration(1) * time.Second) {
		getProcessValue64BufferAndStore()
		zap.S().Debugf("getProcessValue64BufferAndStore called")
	}
}

func getCountBufferAndStore() {
	txn, err := db.Begin()
	if err != nil {
		PQErrorHandling("db.Begin()", err)
	}

	stmt, err := txn.Prepare(pq.CopyIn("counttable", "timestamp", "asset_id", "count", "scrap"))
	if err != nil {
		PQErrorHandling("pq.CopyIn()", err)
	}

	keepRunning := false
	for !keepRunning {
		select {
		case pt := <-countStructChannel:

			timestamp := time.Unix(0, pt.TimestampMs*int64(1000000)).Format("2006-01-02T15:04:05.000Z")
			zap.S().Debugf("getCountBufferAndStore called", pt.TimestampMs, timestamp, pt.DBassetID, pt.Count, pt.Scrap)
			_, err = stmt.Exec(timestamp, pt.DBassetID, pt.Count, pt.Scrap)
			if err != nil {
				PQErrorHandling("stmt.Exec()", err)
			}
		default:
			keepRunning = true
			break
		}
	}

	_, err = stmt.Exec()
	if err != nil {
		PQErrorHandling("stmt.Exec()", err)
	}

	err = stmt.Close()
	if err != nil {
		PQErrorHandling("stmt.Close()", err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", "getCountBufferAndStore")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		return
	}

	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
	}

}

func storeCountBufferIntoDatabase() { //Send the buffer every second into database
	for range time.Tick(time.Duration(1) * time.Second) {
		getCountBufferAndStore()
		zap.S().Debugf("getCountBufferAndStore called")
	}
}

// StoreIntoStateTable stores a state into the database
func StoreIntoStateTable(timestampMs int64, DBassetID int, state int) {
	storeIntoTable(timestampMs, DBassetID, "stateTable", state, "state")
}

// StoreIntoCountTable stores a count into the database
func StoreIntoCountTable(timestampMs int64, DBassetID int, count int, scrap int) {
	zap.S().Debugf("StoreIntoCountTable called", timestampMs, DBassetID, count, scrap)

	countElement := countStruct{
		TimestampMs: timestampMs,
		DBassetID:   DBassetID,
		Count:       count,
		Scrap:       scrap,
	}

	countStructChannel <- countElement

	/*
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			PQErrorHandling("db.BeginTx()", err)
		}

		// WARNING SQL INJECTION POSSIBLE
		sqlStatement := `
			INSERT INTO counttable (timestamp, asset_id, count, scrap)
			VALUES (to_timestamp($1 / 1000.0),$2,$3, $4)
			ON CONFLICT DO NOTHING;`

		_, err = tx.ExecContext(ctx, sqlStatement, timestampMs, DBassetID, count, scrap)
		if err != nil {
			err2 := tx.Rollback()
			if err2 != nil {
				PQErrorHandling("tx.Rollback()", err2)
			}
			PQErrorHandling(sqlStatement, err)
		}

		// if dry run, print statement and rollback
		if isDryRun {
			zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
			err = tx.Rollback()
			if err != nil {
				PQErrorHandling("tx.Rollback()", err)
			}
			return
		}

		err = tx.Commit()
		if err != nil {
			PQErrorHandling("tx.Commit()", err)
		}
	*/
}

// UpdateCountTableWithScrap updates the database to scrap products
func UpdateCountTableWithScrap(timestampMs int64, DBassetID int, scrap int) {
	zap.S().Debugf("UpdateCountTableWithScrap called", timestampMs, DBassetID, scrap)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		UPDATE counttable 
		SET scrap = count 
		WHERE (timestamp, asset_id) IN
			(SELECT timestamp, asset_id
			FROM (
				SELECT *, sum(count) OVER (ORDER BY timestamp DESC) AS running_total
				FROM countTable
				WHERE timestamp < $1 AND timestamp > ($1::TIMESTAMP - INTERVAL '1 DAY') AND asset_id = $2
			) t
			WHERE running_total <= $3)
		;
	`

	// TODO: # 125

	_, err = tx.ExecContext(ctx, sqlStatement, timestampMs, DBassetID, scrap)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// StoreIntoShiftTable stores a count into the database
func StoreIntoShiftTable(timestampMs int64, DBassetID int, timestampMsEnd int64) {
	zap.S().Debugf("StoreIntoShiftTable called", timestampMs, DBassetID, timestampMsEnd)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		INSERT INTO shiftTable (begin_timestamp, end_timestamp, asset_id, type) 
		VALUES (to_timestamp($1 / 1000.0),to_timestamp($2 / 1000.0),$3,$4) 
		ON CONFLICT (begin_timestamp, asset_id) DO UPDATE 
		SET begin_timestamp=to_timestamp($1 / 1000.0), end_timestamp=to_timestamp($2 / 1000.0), asset_id=$3, type=$4;`

	_, err = tx.ExecContext(ctx, sqlStatement, timestampMs, timestampMsEnd, DBassetID, 1) //type is always 1 for now (0 would be no shift)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// StoreIntoUniqueProductTable stores a unique product into the database
func StoreIntoUniqueProductTable(UID string, DBassetID int, timestampMsBegin int64, timestampMsEnd int64, productID string, isScrap bool, qualityClass string, stationID string) {
	zap.S().Debugf("StoreIntoUniqueProductTable called", UID, DBassetID, timestampMsBegin, timestampMsEnd, productID, isScrap, qualityClass, stationID)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		INSERT INTO uniqueProductTable (uid, asset_id, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap, quality_class, station_id) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0),to_timestamp($4 / 1000.0),$5,$6,$7,$8) 
		ON CONFLICT DO NOTHING;`

	_, err = tx.ExecContext(ctx, sqlStatement, UID, DBassetID, timestampMsBegin, timestampMsEnd, productID, isScrap, qualityClass, stationID)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// UpdateUniqueProductTableWithScrap sets isScrap to true in the database
func UpdateUniqueProductTableWithScrap(UID string, DBassetID int) {
	zap.S().Debugf("UpdateUniqueProductTableWithScrap called", UID, DBassetID)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `UPDATE uniqueProductTable SET is_scrap = True WHERE uid = $1 AND asset_id = $2;`

	_, err = tx.ExecContext(ctx, sqlStatement, UID, DBassetID)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// StoreIntoProductTable stores a product into the database
func StoreIntoProductTable(DBassetID int, productName string, timePerUnitInSeconds float64) {
	zap.S().Debugf("StoreIntoProductTable called", DBassetID, productName, timePerUnitInSeconds)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		INSERT INTO productTable (asset_id, product_name, time_per_unit_in_seconds) 
		VALUES ($1, $2, $3) 
		ON CONFLICT DO NOTHING;`

	_, err = tx.ExecContext(ctx, sqlStatement, DBassetID, productName, timePerUnitInSeconds)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// StoreIntoOrderTable stores a order without begin and end timestap into the database
func StoreIntoOrderTable(orderName string, productID int, targetUnits int, DBassetID int) {
	zap.S().Debugf("StoreIntoOrderTable called", orderName, productID, targetUnits, DBassetID)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		INSERT INTO orderTable (order_name, product_id, target_units, asset_id) 
		VALUES ($1, $2, $3, $4) 
		ON CONFLICT DO NOTHING;`

	_, err = tx.ExecContext(ctx, sqlStatement, orderName, productID, targetUnits, DBassetID)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// UpdateBeginTimestampInOrderTable updates an order with a given beginTimestamp
func UpdateBeginTimestampInOrderTable(orderName string, beginTimestamp int64, DBassetID int) {
	zap.S().Debugf("UpdateBeginTimestampInOrderTable called", orderName, beginTimestamp, DBassetID)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		UPDATE orderTable 
		SET begin_timestamp = to_timestamp($1 / 1000.0) 
		WHERE order_name=$2 
			AND asset_id = $3;`

	_, err = tx.ExecContext(ctx, sqlStatement, beginTimestamp, orderName, DBassetID)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// UpdateEndTimestampInOrderTable an order with a given endTimestamp
func UpdateEndTimestampInOrderTable(orderName string, endTimestamp int64, DBassetID int) {
	zap.S().Debugf("UpdateEndTimestampInOrderTable called", orderName, endTimestamp, DBassetID)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		UPDATE orderTable 
		SET end_timestamp = to_timestamp($1 / 1000.0) 
		WHERE order_name=$2 
			AND asset_id = $3;`

	_, err = tx.ExecContext(ctx, sqlStatement, endTimestamp, orderName, DBassetID)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// StoreIntoMaintenancewActivitiesTable stores a new maintenance activity into the database
func StoreIntoMaintenancewActivitiesTable(timestampMs int64, componentID int, activityType int) {
	zap.S().Debugf("StoreIntoMaintenancewActivitiesTable called", timestampMs, componentID, activityType)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
	INSERT INTO maintenanceactivities (component_id, activitytype, timestamp) 
	VALUES ($1, $2, to_timestamp($3 / 1000.0)) 
	ON CONFLICT DO NOTHING;`

	_, err = tx.ExecContext(ctx, sqlStatement, componentID, activityType, timestampMs)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// AddAssetIfNotExisting adds an asset to the db if it is not existing yet
func AddAssetIfNotExisting(assetID string, location string, customerID string) {

	// Get from cache if possible
	var cacheHit bool
	_, cacheHit = internal.GetAssetIDFromCache(customerID, location, assetID)
	if cacheHit { // data found
		zap.S().Debugf("GetAssetID cache hit")
		return
	}

	// Otherwise, add to table

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		PQErrorHandling("db.BeginTx()", err)
	}

	sqlStatement := `
		INSERT INTO assetTable(assetID, location, customer) 
		VALUES ($1,$2,$3) 
		ON CONFLICT DO NOTHING;`

	_, err = tx.ExecContext(ctx, sqlStatement, assetID, location, customerID)
	if err != nil {
		err2 := tx.Rollback()
		if err2 != nil {
			PQErrorHandling("tx.Rollback()", err2)
		}
		PQErrorHandling(sqlStatement, err)
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT", sqlStatement)
		err = tx.Rollback()
		if err != nil {
			PQErrorHandling("tx.Rollback()", err)
		}
		return
	}

	err = tx.Commit()
	if err != nil {
		PQErrorHandling("tx.Commit()", err)
	}
}

// GetAssetID gets the assetID from the database
func GetAssetID(customerID string, location string, assetID string) (DBassetID int) {
	// Get from cache if possible
	var cacheHit bool
	DBassetID, cacheHit = internal.GetAssetIDFromCache(customerID, location, assetID)
	if cacheHit { // data found
		// zap.S().Debugf("GetAssetID cache hit")
		return
	}

	err := db.QueryRow("SELECT id FROM assetTable WHERE assetid=$1 AND location=$2 AND customer=$3;", assetID, location, customerID).Scan(&DBassetID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found")
	} else if err != nil {
		PQErrorHandling("GetAssetID db.QueryRow()", err)
	}

	// Store to cache if not yet existing
	go internal.StoreAssetIDToCache(customerID, location, assetID, DBassetID)
	zap.S().Debugf("Stored AssetID to cache")

	return
}

// GetProductID gets the productID for a asset and a productName from the database
func GetProductID(DBassetID int, productName string) (productID int, err error) {

	err = db.QueryRow("SELECT product_id FROM productTable WHERE asset_id=$1 AND product_name=$2;", DBassetID, productName).Scan(&productID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found", DBassetID, productName)
	} else if err != nil {
		PQErrorHandling("GetProductID db.QueryRow()", err)
	}

	return
}

// GetComponentID gets the componentID from the database
func GetComponentID(assetID int, componentName string) (componentID int) {

	err := db.QueryRow("SELECT id FROM componentTable WHERE asset_id=$1 AND componentName=$2;", assetID, componentName).Scan(&componentID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found")
	} else if err != nil {
		PQErrorHandling("GetComponentID() db.QueryRow()", err)
	}

	return
}
