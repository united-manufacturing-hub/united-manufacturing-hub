package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/beeker1121/goque"
	"github.com/heptiolabs/healthcheck"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/omeid/pgerror"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

var db *sql.DB

var isDryRun bool

const warnStoppingRoutineAsDatabaseHasBeenClosed = "Stopping routine as database has been closed"

// SetupDB setups the db and stores the handler in a global variable in database.go
func SetupDB(PQUser string, PQPassword string, PWDBName string, PQHost string, PQPort int, health healthcheck.Handler, sslmode string, dryRun string) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=%s", PQHost, PQPort, PQUser, PQPassword, PWDBName, sslmode)
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
	db.SetMaxOpenConns(20)
	// Healthcheck
	health.AddReadinessCheck("database", healthcheck.DatabasePingCheck(db, 1*time.Second))

}

// ShutdownDB closes all database connections
func ShutdownDB() {
	err := db.Close()
	if err != nil {
		panic(err)
	}
}

// PQErrorHandlingTransaction logs and handles postgresql errors in transactions
func PQErrorHandlingTransaction(sqlStatement string, err error, txn *sql.Tx) (returnedErr error) {
	PQErrorHandling(sqlStatement, err)

	if e := pgerror.UniqueViolation(err); e != nil {
		zap.S().Warnf("PostgreSQL failed: UniqueViolation", err, sqlStatement)
		return
	} else if e := pgerror.CheckViolation(err); e != nil {
		zap.S().Warnf("PostgreSQL failed: CheckViolation", err, sqlStatement)
		return
	}

	zap.S().Warnf("PostgreSQL error: ", err, sqlStatement)

	err2 := txn.Rollback()
	if err2 != nil {
		PQErrorHandling("txn.Rollback()", err2)
	}
	returnedErr = err
	return
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

// storeIntoDatabaseRoutineRecommendation fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineRecommendation(pg *goque.PrefixQueue) {
	processQueue(pg, prefixRecommendation, storeItemsIntoDatabaseRecommendation)
}

func processQueue(pg *goque.PrefixQueue, prefix string, f func(itemArray []goque.Item) (err error)) {
	for range time.Tick(time.Duration(1) * time.Second) {

		// GetItemsFromQueue

		itemArray, err := getAllItemsInQueue(prefix, pg)
		if err == goque.ErrDBClosed {
			zap.S().Warnf(warnStoppingRoutineAsDatabaseHasBeenClosed, prefix)
			return
		} else if err != nil {
			zap.S().Errorf("Failed to get items from database", prefix)
			continue
		}

		if len(itemArray) == 0 {
			//zap.S().Debugf("Queue empty", prefix)
			continue
		}

		zap.S().Debugf("Got items from queue", prefix, len(itemArray))

		err = f(itemArray)
		if err != nil {
			zap.S().Errorf("Failed to store items in database", prefix)
			addMultipleItemsToQueue(prefix, pg, itemArray)
		}
	}
}

func storeItemsIntoDatabaseRecommendation(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt
	stmt, err = txn.Prepare(`
		INSERT INTO recommendationTable (timestamp, uid, recommendationType, enabled, recommendationValues, recommendationTextEN, recommendationTextDE, diagnoseTextEN, diagnoseTextDE) 
		VALUES (to_timestamp($1 / 1000.0),$2,$3,$4,$5,$6,$7,$8,$9) 
		ON CONFLICT (uid) DO UPDATE 
		SET timestamp=to_timestamp($1 / 1000.0), uid=$2, recommendationType=$3, enabled=$4, recommendationValues=$5, recommendationTextEN=$6, recommendationTextDE=$7, diagnoseTextEN=$8, diagnoseTextDE=$9;`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt recommendationStruct

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.UID, pt.RecommendationType, pt.Enabled, pt.RecommendationValues, pt.RecommendationTextEN, pt.RecommendationTextDE, pt.DiagnoseTextEN, pt.DiagnoseTextDE)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}

	return

}

func storeItemsIntoDatabaseProcessValueFloat64(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
	}

	// 1. Prepare statement: create temp table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			CREATE TEMP TABLE tmp_processvaluetable64 
				( LIKE processValueTable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`)
		if err != nil {
			err = PQErrorHandlingTransaction("Prepare()", err, txn)
		}

		// Create statement
		_, err = stmt.Exec()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

		// Close Statement
		err = stmt.Close()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
			if err != nil {
				return
			}
		}

	}
	// 2. Prepare statement: copying into temp table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(pq.CopyIn("tmp_processvaluetable64", "timestamp", "asset_id", "value", "valuename"))
		if err != nil {
			err = PQErrorHandlingTransaction("Prepare()", err, txn)
			if err != nil {
				return
			}
		}

		for _, item := range itemArray {

			var pt processValueFloat64Queue

			err = item.ToObject(&pt)

			if err != nil {
				err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
				if err != nil {
					return
				}
			}

			timestamp := time.Unix(0, pt.TimestampMs*int64(1000000)).Format("2006-01-02T15:04:05.000Z")

			// Create statement
			_, err = stmt.Exec(timestamp, pt.DBAssetID, pt.Value, pt.Name)
			if err != nil {
				err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
				if err != nil {
					return
				}
			}

		}

		// Close Statement
		err = stmt.Close()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// 3. Prepare statement: copy from temp table into main table
	{

		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			INSERT INTO processvaluetable (SELECT * FROM tmp_processvaluetable64) ON CONFLICT DO NOTHING;
		`)
		if err != nil {
			PQErrorHandling("Prepare()", err)
			if err != nil {
				return
			}
		}

		// Create statement
		_, err = stmt.Exec()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

		// Close Statement
		err = stmt.Close()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
	}
	return
}

// storeIntoDatabaseRoutineProcessValueFloat64 fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineProcessValueFloat64(pg *goque.PrefixQueue) {
	processQueue(pg, prefixProcessValueFloat64, storeItemsIntoDatabaseProcessValueFloat64)
}

func storeItemsIntoDatabaseProcessValue(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
	}

	// 1. Prepare statement: create temp table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			CREATE TEMP TABLE tmp_processvaluetable 
				( LIKE processValueTable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`)
		if err != nil {
			err = PQErrorHandlingTransaction("Prepare()", err, txn)
		}

		// Create statement
		_, err = stmt.Exec()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

		// Close Statement
		err = stmt.Close()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
			if err != nil {
				return
			}
		}

	}
	// 2. Prepare statement: copying into temp table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(pq.CopyIn("tmp_processvaluetable", "timestamp", "asset_id", "value", "valuename"))
		if err != nil {
			err = PQErrorHandlingTransaction("Prepare()", err, txn)
			if err != nil {
				return
			}
		}

		for _, item := range itemArray {

			var pt processValueQueue

			err = item.ToObject(&pt)

			if err != nil {
				err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
				if err != nil {
					return
				}
			}

			timestamp := time.Unix(0, pt.TimestampMs*int64(1000000)).Format("2006-01-02T15:04:05.000Z")

			// Create statement
			_, err = stmt.Exec(timestamp, pt.DBAssetID, pt.Value, pt.Name)
			if err != nil {
				err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
				if err != nil {
					return
				}
			}

		}

		// Close Statement
		err = stmt.Close()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// 3. Prepare statement: copy from temp table into main table
	{

		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			INSERT INTO processvaluetable (SELECT * FROM tmp_processvaluetable) ON CONFLICT DO NOTHING;
		`)
		if err != nil {
			PQErrorHandling("Prepare()", err)
			if err != nil {
				return
			}
		}

		// Create statement
		_, err = stmt.Exec()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

		// Close Statement
		err = stmt.Close()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
	}
	return
}

// storeIntoDatabaseRoutineProcessValue fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineProcessValue(pg *goque.PrefixQueue) {
	processQueue(pg, prefixProcessValue, storeItemsIntoDatabaseProcessValue)
}


// storeIntoDatabaseRoutineCount fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineCount(pg *goque.PrefixQueue) {
	processQueue(pg, prefixCount, storeItemsIntoDatabaseCount)
}

func storeItemsIntoDatabaseCount(itemArray []goque.Item) (err error) {
	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
	}

	// 1. Prepare statement: create temp table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			CREATE TEMP TABLE tmp_counttable
				( LIKE counttable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`)
		if err != nil {
			err = PQErrorHandlingTransaction("Prepare()", err, txn)
		}

		// Create statement
		_, err = stmt.Exec()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

		// Close Statement
		err = stmt.Close()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
			if err != nil {
				return
			}
		}

	}
	// 2. Prepare statement: copying into temp table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(pq.CopyIn("tmp_counttable", "timestamp", "asset_id", "count", "scrap"))
		if err != nil {
			err = PQErrorHandlingTransaction("Prepare()", err, txn)
			if err != nil {
				return
			}
		}

		for _, item := range itemArray {

			var pt countQueue

			err = item.ToObject(&pt)

			if err != nil {
				err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
				if err != nil {
					return
				}
			}

			timestamp := time.Unix(0, pt.TimestampMs*int64(1000000)).Format("2006-01-02T15:04:05.000Z")

			// Create statement
			_, err = stmt.Exec(timestamp, pt.DBAssetID, pt.Count, pt.Scrap)
			if err != nil {
				err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
				if err != nil {
					return
				}
			}

		}

		// Close Statement
		err = stmt.Close()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// 3. Prepare statement: copy from temp table into main table
	{

		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			INSERT INTO counttable (SELECT * FROM tmp_counttable) ON CONFLICT DO NOTHING;
		`)
		if err != nil {
			PQErrorHandling("Prepare()", err)
			if err != nil {
				return
			}
		}

		// Create statement
		_, err = stmt.Exec()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

		// Close Statement
		err = stmt.Close()
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
	}
	return
}


// storeIntoDatabaseRoutineState fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineState(pg *goque.PrefixQueue) {
	processQueue(pg, prefixState, storeItemsIntoDatabaseState)
}

func storeItemsIntoDatabaseState(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt
	stmt, err = txn.Prepare(`
		INSERT INTO statetable (timestamp, asset_id, state) 
		VALUES (to_timestamp($1 / 1000.0),$2,$3)`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt stateQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.DBAssetID, pt.State)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineScrapCount from queue and sends it to the database
func storeIntoDatabaseRoutineScrapCount(pg *goque.PrefixQueue) {
	processQueue(pg, prefixScrapCount, storeItemsIntoDatabaseScrapCount)
}

func storeItemsIntoDatabaseScrapCount(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt

	stmt, err = txn.Prepare(`
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
		;`)

	// TODO: # 125
	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt scrapCountQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.DBAssetID, pt.Scrap)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineUniqueProduct fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineUniqueProduct(pg *goque.PrefixQueue) {
	processQueue(pg, prefixUniqueProduct, storeItemsIntoDatabaseUniqueProduct)
}

func storeItemsIntoDatabaseUniqueProduct(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt
	stmt, err = txn.Prepare(`
		INSERT INTO uniqueProductTable (asset_id, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap, uniqueProductAlternativeID) 
		VALUES ($1, to_timestamp($2 / 1000.0),to_timestamp($3 / 1000.0),$4,$5,$6) 
		ON CONFLICT DO NOTHING;`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt uniqueProductQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.DBAssetID, pt.BeginTimestampMs, NewNullInt64(pt.EndTimestampMs), pt.ProductID, pt.IsScrap, pt.UniqueProductAlternativeID)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineProductTag fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineProductTag(pg *goque.PrefixQueue) {
	processQueue(pg, prefixProductTag, storeItemsIntoDatabaseProductTag)
}

func storeItemsIntoDatabaseProductTag(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt
	stmt, err = txn.Prepare(`
		INSERT INTO productTagTable (valueName, value, timestamp, product_uid) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0), $4) 
		ON CONFLICT DO NOTHING;`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {
		var pt productTagQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}

		var uid int
		uid, err = GetUniqueProductID(pt.AID, pt.DBAssetID)
		if err != nil {
			zap.S().Errorf("Stopped writing productTag in Database, uid not found")
			return
		}
		// Create statement
		_, err = stmt.Exec(pt.Name, pt.Value, pt.TimestampMs, uid)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineProductTagString fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineProductTagString(pg *goque.PrefixQueue) {
	processQueue(pg, prefixProductTagString, storeItemsIntoDatabaseProductTagString)
}

func storeItemsIntoDatabaseProductTagString(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt
	stmt, err = txn.Prepare(`
		INSERT INTO productTagStringTable (valueName, value, timestamp, product_uid) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0), $4) 
		ON CONFLICT DO NOTHING;`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {
		var pt productTagStringQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}

		var uid int
		uid, err = GetUniqueProductID(pt.AID, pt.DBAssetID)
		if err != nil {
			zap.S().Errorf("Stopped writing productTagString in Database, uid not found")
			return
		}
		// Create statement
		_, err = stmt.Exec(pt.Name, pt.Value, pt.TimestampMs, uid)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineAddParentToChild fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineAddParentToChild(pg *goque.PrefixQueue) {
	processQueue(pg, prefixAddParentToChild, storeItemsIntoDatabaseAddParentToChild)
}

func storeItemsIntoDatabaseAddParentToChild(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt
	stmt, err = txn.Prepare(`
		INSERT INTO productInheritanceTable (parent_uid, child_uid, timestamp) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0)) 
		ON CONFLICT DO NOTHING;`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {
		var pt addParentToChildQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}

		var childUid int
		childUid, err = GetUniqueProductID(pt.ChildAID, pt.DBAssetID)
		if err != nil {
			zap.S().Errorf("Stopped writing addParentToChild in Database, childUid not found")
			return
		}
		var parentUid = GetLatestParentUniqueProductID(pt.ParentAID, pt.DBAssetID)
		// Create statement
		_, err = stmt.Exec(parentUid, childUid, pt.TimestampMs)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineShift fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineShift(pg *goque.PrefixQueue) {
	processQueue(pg, prefixAddShift, storeItemsIntoDatabaseShift)
}

func storeItemsIntoDatabaseShift(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt
	stmt, err = txn.Prepare(`
		INSERT INTO shiftTable (begin_timestamp, end_timestamp, asset_id, type) 
		VALUES (to_timestamp($1 / 1000.0),to_timestamp($2 / 1000.0),$3,$4) 
		ON CONFLICT (begin_timestamp, asset_id) DO UPDATE 
		SET begin_timestamp=to_timestamp($1 / 1000.0), end_timestamp=to_timestamp($2 / 1000.0), asset_id=$3, type=$4;`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt addShiftQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.TimestampMsEnd, pt.DBAssetID, 1) //type is always 1 for now (0 would be no shift)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineUniqueProductScrap fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineUniqueProductScrap(pg *goque.PrefixQueue) {
	processQueue(pg, prefixUniqueProductScrap, storeItemsIntoDatabaseUniqueProductScrap)
}

func storeItemsIntoDatabaseUniqueProductScrap(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt

	stmt, err = txn.Prepare(`UPDATE uniqueProductTable SET is_scrap = True WHERE uniqueProductID = $1 AND asset_id = $2;`)
	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt scrapUniqueProductQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.UID, pt.DBAssetID)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineAddProduct fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineAddProduct(pg *goque.PrefixQueue) {
	processQueue(pg, prefixAddProduct, storeItemsIntoDatabaseAddProduct)
}

func storeItemsIntoDatabaseAddProduct(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt

	stmt, err = txn.Prepare(`INSERT INTO productTable (asset_id, product_name, time_per_unit_in_seconds) 
		VALUES ($1, $2, $3) 
		ON CONFLICT DO NOTHING;`)
	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt addProductQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.DBAssetID, pt.ProductName, pt.TimePerUnitInSeconds)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineAddOrder fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineAddOrder(pg *goque.PrefixQueue) {
	processQueue(pg, prefixAddOrder, storeItemsIntoDatabaseAddOrder)
}

func storeItemsIntoDatabaseAddOrder(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt

	stmt, err = txn.Prepare(`INSERT INTO orderTable (order_name, product_id, target_units, asset_id) 
		VALUES ($1, $2, $3, $4) 
		ON CONFLICT DO NOTHING;`)
	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt addOrderQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.OrderName, pt.ProductID, pt.TargetUnits, pt.DBAssetID)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineStartOrder fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineStartOrder(pg *goque.PrefixQueue) {
	processQueue(pg, prefixStartOrder, storeItemsIntoDatabaseStartOrder)
}

func storeItemsIntoDatabaseStartOrder(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt

	stmt, err = txn.Prepare(`UPDATE orderTable 
		SET begin_timestamp = to_timestamp($1 / 1000.0) 
		WHERE order_name=$2 
			AND asset_id = $3;`)
	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt startOrderQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.OrderName, pt.DBAssetID)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineEndOrder fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineEndOrder(pg *goque.PrefixQueue) {
	processQueue(pg, prefixEndOrder, storeItemsIntoDatabaseEndOrder)
}

func storeItemsIntoDatabaseEndOrder(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt

	stmt, err = txn.Prepare(`UPDATE orderTable 
		SET end_timestamp = to_timestamp($1 / 1000.0) 
		WHERE order_name=$2 
			AND asset_id = $3;`)
	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt endOrderQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.OrderName, pt.DBAssetID)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

// storeIntoDatabaseRoutineAddMaintenanceActivity fetches data from queue and sends it to the database
func storeIntoDatabaseRoutineAddMaintenanceActivity(pg *goque.PrefixQueue) {
	processQueue(pg, prefixAddMaintenanceActivity, storeItemsIntoDatabaseAddMaintenanceActivity)
}

func storeItemsIntoDatabaseAddMaintenanceActivity(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt

	stmt, err = txn.Prepare(`INSERT INTO maintenanceactivities (component_id, activitytype, timestamp) 
	VALUES ($1, $2, to_timestamp($3 / 1000.0)) 
	ON CONFLICT DO NOTHING;`)
	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt addMaintenanceActivityQueue

		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}
		// Create statement
		_, err = stmt.Exec(pt.ComponentID, pt.Activity, pt.TimestampMs)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}

	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}
	return

}

func modifyInDatabaseRoutineModifyState(pg *goque.PrefixQueue) {
	processQueue(pg, prefixModifyState, modifyStateInDatabase)
}

func modifyStateInDatabase(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var StmtGetLastInRange *sql.Stmt
	StmtGetLastInRange, err = txn.Prepare(`SELECT extract(epoch from timestamp)*1000, asset_id, state FROM statetable WHERE timestamp > to_timestamp($1 / 1000.0) AND asset_id = $2 ORDER BY timestamp ASC LIMIT 1;`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	var StmtDeleteInRange *sql.Stmt
	StmtDeleteInRange, err = txn.Prepare(`DELETE FROM statetable WHERE timestamp >= to_timestamp($1 / 1000.0) AND timestamp <= to_timestamp($2 / 1000.0) AND asset_id = $3;`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	var StmtInsertNewState *sql.Stmt
	StmtInsertNewState, err = txn.Prepare(`INSERT INTO statetable (timestamp, asset_id, state) VALUES (to_timestamp($1 / 1000.0),$2,$3);`)
	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	var StmtDeleteOldState *sql.Stmt
	StmtDeleteOldState, err = txn.Prepare(`DELETE FROM statetable WHERE timestamp = to_timestamp($1 / 1000.0);`)
	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {
		var pt modifyStateQueue
		err = item.ToObject(&pt)
		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}

		//Get last row in time range
		var val *sql.Rows
		val, err = StmtGetLastInRange.Query(pt.StartTimeStamp, pt.DBAssetID)
		if err != nil {
			err = PQErrorHandlingTransaction("StmtGetLastInRange.Exec()", err, txn)
			if err != nil {
				return
			}
		}

		zap.S().Debugf("Looking up", pt.StartTimeStamp, pt.EndTimeStamp, pt.DBAssetID)

		//If there is a row inside timeframe
		if val.Next() {
			zap.S().Debugf("Got NextResultSet")
			var (
				LastRowTimestamp    float64
				LastRowTimestampInt int64
				LastRowAssetId      int64
				LastRowState        int64
			)
			err = val.Scan(&LastRowTimestamp, &LastRowAssetId, &LastRowState)
			if err != nil {
				err = PQErrorHandlingTransaction("rows.Scan()", err, txn)
				if err != nil {
					return
				}
			}
			LastRowTimestampInt = int64(int(LastRowTimestamp))
			zap.S().Debugf("Row: ", LastRowState, LastRowTimestampInt, LastRowAssetId)

			err = val.Close()
			if err != nil {
				return
			}

			//Delete all rows inside timeframe
			zap.S().Debugf("DeleteState: ", pt.StartTimeStamp, pt.EndTimeStamp, pt.DBAssetID)
			_, err = StmtDeleteInRange.Exec(pt.StartTimeStamp, pt.EndTimeStamp, pt.DBAssetID)
			if err != nil {
				err = PQErrorHandlingTransaction("StmtDeleteInRange.Exec()", err, txn)
				if err != nil {
					return
				}
			}

			zap.S().Debugf("NewState: ", pt.StartTimeStamp, pt.DBAssetID, pt.NewState)
			//Insert new state
			_, err = StmtInsertNewState.Exec(pt.StartTimeStamp, pt.DBAssetID, pt.NewState)
			if err != nil {
				err = PQErrorHandlingTransaction("StmtInsertNewState.Exec()", err, txn)
				if err != nil {
					return
				}
			}

			//Update old state, inside timeframe with new begins
			zap.S().Debugf("UpdateState: ", pt.EndTimeStamp, LastRowTimestampInt, pt.DBAssetID, LastRowState)
			_, err = StmtDeleteOldState.Exec(LastRowTimestampInt)
			if err != nil {
				err = PQErrorHandlingTransaction("StmtInsertNewState.Exec()", err, txn)
				if err != nil {
					return
				}
			}
			_, err = StmtInsertNewState.Exec(pt.EndTimeStamp, pt.DBAssetID, LastRowState)
			if err != nil {
				err = PQErrorHandlingTransaction("StmtInsertNewState.Exec()", err, txn)
				if err != nil {
					return
				}
			}

		} else {
			zap.S().Debugf("No Rows !")
			//No old state in timeframe, just insert new state
			_, err = StmtInsertNewState.Exec(pt.StartTimeStamp, pt.DBAssetID, pt.NewState)
			if err != nil {
				err = PQErrorHandlingTransaction("StmtInsertNewState.Exec()", err, txn)
				if err != nil {
					return
				}
			}
		}

	}

	// Close Statement
	err = StmtGetLastInRange.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("StmtGetLastInRange.Close()", err, txn)
		if err != nil {
			return
		}
	}
	err = StmtDeleteInRange.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("StmtDeleteInRange.Close()", err, txn)
		if err != nil {
			return
		}
	}
	err = StmtInsertNewState.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("StmtInsertNewState.Close()", err, txn)
		if err != nil {
			return
		}
	}

	err = StmtDeleteOldState.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("StmtDeleteOldState.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}

	return
}

func deleteInDatabaseRoutineDeleteShiftById(pg *goque.PrefixQueue) {
	processQueue(pg, prefixDeleteShiftById, deleteShiftInDatabaseById)
}

func deleteShiftInDatabaseById(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt
	stmt, err = txn.Prepare(`DELETE FROM shifttable WHERE id = $1;`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt deleteShiftByIdQueue
		err = item.ToObject(&pt)
		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}

		_, err = stmt.Exec(pt.ShiftId)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}
	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}

	return
}

func deleteInDatabaseRoutineDeleteShiftByAssetIdAndTimestamp(pg *goque.PrefixQueue) {
	processQueue(pg, prefixDeleteShiftByAssetIdAndBeginTimestamp, deleteShiftInDatabaseByAssetIdAndTimestamp)
}

func deleteShiftInDatabaseByAssetIdAndTimestamp(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmt *sql.Stmt
	stmt, err = txn.Prepare(`DELETE FROM shifttable WHERE asset_id = $1 AND begin_timestamp = to_timestamp($2 / 1000.0);`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {

		var pt deleteShiftByAssetIdAndBeginTimestampQueue
		err = item.ToObject(&pt)
		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}

		_, err = stmt.Exec(pt.DBAssetID, pt.BeginTimeStamp)
		if err != nil {
			err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
			if err != nil {
				return
			}
		}

	}
	// Close Statement
	err = stmt.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmt.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}

	return
}

// AddAssetIfNotExisting adds an asset to the db if it is not existing yet
func AddAssetIfNotExisting(assetID string, location string, customerID string) {
	// Get from cache if possible
	var cacheHit bool
	_, cacheHit = internal.GetAssetIDFromCache(customerID, location, assetID)
	if cacheHit { // data found
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

func modifyInDatabaseRoutineModifyCountAndScrap(pg *goque.PrefixQueue) {
	processQueue(pg, prefixModifyProducesPiece, modifyInDatabaseModifyCountAndScrap)
}

func modifyInDatabaseModifyCountAndScrap(itemArray []goque.Item) (err error) {

	// Begin transaction
	txn, err := db.Begin()

	if err != nil {
		err = PQErrorHandlingTransaction("db.Begin()", err, txn)
		if err != nil {
			return
		}
	}

	var stmtCS *sql.Stmt
	stmtCS, err = txn.Prepare(`UPDATE counttable SET count = $1, scrap = $2 WHERE asset_id = $3`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	var stmtC *sql.Stmt
	stmtC, err = txn.Prepare(`UPDATE counttable SET count = $1 WHERE asset_id = $2`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	var stmtS *sql.Stmt
	stmtS, err = txn.Prepare(`UPDATE counttable SET scrap = $1 WHERE asset_id = $2`)

	if err != nil {
		PQErrorHandling("Prepare()", err)
		if err != nil {
			return
		}
	}

	for _, item := range itemArray {
		var pt modifyProducesPieceQueue
		err = item.ToObject(&pt)

		if err != nil {
			err = PQErrorHandlingTransaction("item.ToObject()", err, txn)
			if err != nil {
				return
			}
		}

		// pt.Count is -1, if not modified by user
		if pt.Count != -1 {
			// pt.Scrap is -1, if not modified by user
			if pt.Scrap != -1 {
				zap.S().Debugf("CS !", pt.Count, pt.Scrap, pt.DBAssetID)
				_, err = stmtCS.Exec(pt.Count, pt.Scrap, pt.DBAssetID)
				if err != nil {
					err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
					if err != nil {
						return
					}
				}
			} else {
				zap.S().Debugf("C !", pt.Count, pt.DBAssetID)
				_, err = stmtC.Exec(pt.Count, pt.DBAssetID)
				if err != nil {
					err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
					if err != nil {
						return
					}
				}
			}
		} else {
			// pt.Scrap is -1, if not modified by user
			if pt.Scrap != -1 {
				zap.S().Debugf("S !", pt.Scrap, pt.DBAssetID)
				_, err = stmtS.Exec(pt.Scrap, pt.DBAssetID)
				if err != nil {
					err = PQErrorHandlingTransaction("stmt.Exec()", err, txn)
					if err != nil {
						return
					}
				}
			} else {
				zap.S().Errorf("Invalid amount for Count and Script")
			}
		}
	}

	// Close Statement
	err = stmtC.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmtC.Close()", err, txn)
		if err != nil {
			return
		}
	}
	err = stmtCS.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmtCS.Close()", err, txn)
		if err != nil {
			return
		}
	}
	err = stmtS.Close()
	if err != nil {
		err = PQErrorHandlingTransaction("stmtS.Close()", err, txn)
		if err != nil {
			return
		}
	}

	// if dry run, print statement and rollback
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}

	// Commit all statements
	err = txn.Commit()
	if err != nil {
		PQErrorHandling("txn.Commit()", err)
		if err != nil {
			return
		}
	}

	return
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

func GetUniqueProductID(aid string, DBassetID int) (uid int, err error) {

	uid, cacheHit := internal.GetUniqueProductIDFromCache(aid, DBassetID)
	if !cacheHit { // data NOT found
		err = db.QueryRow("SELECT uniqueProductID FROM uniqueProductTable WHERE uniqueProductAlternativeID = $1 AND asset_id = $2;", aid, DBassetID).Scan(&uid)
		if err == sql.ErrNoRows {
			zap.S().Errorf("No Results Found", aid, DBassetID)
		} else if err != nil {
			PQErrorHandling("GetUniqueProductID db.QueryRow()", err)
		}
		internal.StoreUniqueProductIDToCache(aid, DBassetID, uid)
	}

	return
}

func GetLatestParentUniqueProductID(aid string, assetID int) (uid int) {

	err := db.QueryRow("SELECT uniqueProductID FROM uniqueProductTable WHERE uniqueProductAlternativeID = $1 AND NOT asset_id = $2 ORDER BY begin_timestamp_ms DESC LIMIT 1;",
		aid, assetID).Scan(&uid)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found", aid, assetID)
	} else if err != nil {
		PQErrorHandling("GetLatestParentUniqueProductID db.QueryRow()", err)
	}
	return
}

// NewNullInt64 returns sql.NullInt64: {0 false} if i == 0 and  {<i> true} if i != 0
func NewNullInt64(i int64) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{
		Int64: i,
		Valid: true,
	}
}
