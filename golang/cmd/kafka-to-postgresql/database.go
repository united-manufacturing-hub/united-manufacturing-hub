package main

import (
	"database/sql"
	"fmt"
	"github.com/heptiolabs/healthcheck"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"regexp"
	"strings"
	"time"
)

var db *sql.DB
var statement *StatementRegistry

var isDryRun bool

// SetupDB setups the db and stores the handler in a global variable in database.go
func SetupDB(PQUser string, PQPassword string, PWDBName string, PQHost string, PQPort int, health healthcheck.Handler, dryRun string, sslmode string) {

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
	var ok bool
	var perr error
	if ok, perr = IsPostgresSQLAvailable(); !ok {
		panic(fmt.Sprintf("Postgres not yet available: %s", perr))
	}

	db.SetMaxOpenConns(20)
	// Healthcheck
	health.AddReadinessCheck("database", healthcheck.DatabasePingCheck(db, 1*time.Second))

	statement = NewStatementRegistry()
}

//IsPostgresSQLAvailable returns if the database is reachable by PING command
func IsPostgresSQLAvailable() (bool, error) {
	var err error
	if db != nil {
		err = db.Ping()
		if err == nil {
			return true, nil
		}
	}
	return false, err
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

type RecoveryType int32

const (
	Other        RecoveryType = 0
	DatabaseDown RecoveryType = 1
	DiscardValue RecoveryType = 2
)

//GetPostgresErrorRecoveryOptions checks if the error is recoverable
func GetPostgresErrorRecoveryOptions(err error) RecoveryType {

	// Why go allows returning errors, that are not exported is still beyond me
	errorString := err.Error()
	isRecoverableByRetrying := strings.Contains(errorString, "sql: database is closed") ||
		strings.Contains(errorString, "driver: bad connection") ||
		strings.Contains(errorString, "connect: connection refused") ||
		strings.Contains(errorString, "pq: the database system is shutting down") ||
		strings.Contains(errorString, "connect: no route to host")
	if isRecoverableByRetrying {
		return DatabaseDown
	}

	matchedOutOfRange, err := regexp.MatchString(`pq: value "-*\d+" is out of range for type integer`, errorString)

	isRecoverableByDiscarding := matchedOutOfRange
	if isRecoverableByDiscarding {
		return DiscardValue
	}
	return Other
}

// GetAssetTableID gets the assetID from the database
func GetAssetTableID(customerID string, location string, assetID string) (AssetTableID uint32, success bool) {
	zap.S().Debugf("[GetAssetTableID] customerID: %s, location: %s, assetID: %s", customerID, location, assetID)

	success = false
	// Get from cache if possible
	var cacheHit bool
	AssetTableID, cacheHit = GetCacheAssetTableId(customerID, location, assetID)
	if cacheHit {
		zap.S().Debugf("[GetAssetTableID] Cache hit for customerID: %s, location: %s, assetID: %s", customerID, location, assetID)
		success = true
		return
	}

	err := statement.SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId.QueryRow(assetID, location, customerID).Scan(&AssetTableID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found for assetID: %s, location: %s, customerID: %s", assetID, location, customerID)
	} else if err != nil {
		zap.S().Debugf("[GetAssetTableID] Error: %s", err)
		switch GetPostgresErrorRecoveryOptions(err) {
		case DiscardValue:
			return 0, false
		case DatabaseDown:
			return 0, false
		case Other:
			return 0, false
		}
		return
	}

	// Store to cache if not yet existing
	go PutCacheAssetTableId(customerID, location, assetID, AssetTableID)
	zap.S().Debugf("Stored AssetID to cache")

	success = true
	return
}
