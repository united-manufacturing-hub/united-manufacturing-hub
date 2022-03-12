package main

import (
	"database/sql"
	"fmt"
	"github.com/heptiolabs/healthcheck"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"time"
)

var db *sql.DB
var statement *statementRegistry

var isDryRun bool

// SetupDB setups the db and stores the handler in a global variable in database.go
func SetupDB(PQUser string, PQPassword string, PWDBName string, PQHost string, PQPort int, health healthcheck.Handler, dryRun string) {

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=require", PQHost, PQPort, PQUser, PQPassword, PWDBName)
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

	statement = newStatementRegistry()
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
