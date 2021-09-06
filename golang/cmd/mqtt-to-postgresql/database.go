package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/heptiolabs/healthcheck"
	_ "github.com/lib/pq"
	"github.com/omeid/pgerror"
	"go.uber.org/zap"
	"time"
)

var db *sql.DB
var statement *statementRegistry

var isDryRun bool

// SetupDB setups the db and stores the handler in a global variable in database.go
func SetupDB(PQUser string, PQPassword string, PWDBName string, PQHost string, PQPort int, health healthcheck.Handler, sslmode string, dryRun string) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=%s", PQHost, PQPort, PQUser, PQPassword, PWDBName, sslmode)
	var err error
	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
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

	statement = newStatementRegistry()
}

// ShutdownDB closes all database connections
func ShutdownDB() {
	err := statement.Shutdown()
	if err != nil {
		panic(err)
	}
	err = db.Close()
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

func deferCallback(txn *sql.Tx) (err error) {
	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		err = txn.Rollback()
		if err != nil {
			PQErrorHandling("txn.Rollback()", err)
		}
		if err != nil {
			return
		}
	}else{
		err = txn.Commit()
		if err != nil {
			PQErrorHandling("txn.Commit()", err)
			if err != nil {
				return
			}
		}
	}
	return
}

func storeItemsIntoDatabaseRecommendation(items []QueueObject) (faultyItems []QueueObject, err error) {
	txn, err := db.Begin()
	if err != nil {
		faultyItems = items
		return
	}

	defer func() {
		err = deferCallback(txn)
		if err != nil {
			return
		}
	}()

	for _, item := range items {
		var pt recommendationStruct
		err = json.Unmarshal(item.Payload, &pt)
		if err != nil {
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		//These statements' auto close
		stmt := txn.Stmt(statement.InsertIntoRecommendationTable)

		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.UID, pt.RecommendationType, pt.Enabled, pt.RecommendationValues, pt.RecommendationTextEN, pt.RecommendationTextDE, pt.DiagnoseTextEN, pt.DiagnoseTextDE)
		if err != nil {
			faultyItems = append(faultyItems, item)
			continue
		}
	}
	return
}



func storeItemsIntoDatabaseProcessValueFloat64(items []QueueObject) (faultyItems []QueueObject, err error) {
	txn, err := db.Begin()
	if err != nil {
		faultyItems = items
		return
	}

	defer func() {
		err = deferCallback(txn)
		if err != nil {
			return
		}
	}()


	// 1. Prepare statement: create temp table
	//These statements' auto close
	{
		stmt := txn.Stmt(statement.CreateTmpProcessValueTable64)
		_, err = stmt.Exec()
		if err != nil {
			faultyItems = items
			return
		}
	}
	// 2. Prepare statement: copying into temp table
	{
		stmt := txn.Stmt(statement.CopyInTmpProcessValueTable64)
		for _, item := range items {
			var pt processValueFloat64Queue
			err = json.Unmarshal(item.Payload, &pt)
			if err != nil {
				zap.S().Errorf("Failed to unmarshal item", item)
				continue
			}

			timestamp := time.Unix(0, pt.TimestampMs*int64(1000000)).Format("2006-01-02T15:04:05.000Z")

			_, err = stmt.Exec(timestamp, pt.DBAssetID, pt.Value, pt.Name)
			if err != nil {
				faultyItems = append(faultyItems, item)
				continue
			}
		}
	}

	// 3. Prepare statement: copy from temp table into main table
	{
		//These statements' auto close
		stmt := txn.Stmt(statement.InsertIntoProcessValueTableFromTmpProcessValueTable64)
		_, err = stmt.Exec()
		if err != nil {
			faultyItems = items
			return
		}
	}
	return
}