package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

// SetupDB setups the db and stores the handler in a global variable in database.go
func SetupDB(
	PQUser string,
	PQPassword string,
	PWDBName string,
	PQHost string,
	PQPort int,
	health healthcheck.Handler,
	sslmode string) *sql.DB {

	psqlInfo := fmt.Sprintf(
		"host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=%s",
		PQHost,
		PQPort,
		PQUser,
		PQPassword,
		PWDBName,
		sslmode)
	var db *sql.DB
	var err error
	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		zap.S().Fatalf("Error opening database: %s", err)
	}

	var ok bool
	if ok, err = IsPostgresSQLAvailable(db); !ok {
		zap.S().Fatalf("Postgres not yet available: %s", err)
	}

	db.SetMaxOpenConns(20)

	// Healthcheck
	health.AddReadinessCheck("database", healthcheck.DatabasePingCheck(db, internal.OneSecond))

	health.AddLivenessCheck("database", healthcheck.DatabasePingCheck(db, 30*time.Second))

	return db
}

// IsPostgresSQLAvailable returns if the database is reachable by PING command
func IsPostgresSQLAvailable(db *sql.DB) (bool, error) {
	var err error
	if db != nil {
		ctx, ctxClose := context.WithTimeout(context.Background(), internal.FiveSeconds)
		defer ctxClose()
		err = db.PingContext(ctx)
		if err == nil {
			return true, nil
		}
	}
	return false, err
}

// ShutdownDB closes all database connections
func ShutdownDB(db *sql.DB) {

	zap.S().Infof("Closing database connection")

	if err := db.Close(); err != nil {
		zap.S().Fatalf("Error closing database: %s", err)
	}
}
