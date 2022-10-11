package database

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"strings"
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
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
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

func CheckIfColumnExists(colName, tableName string, db *sql.DB) (bool, error) {
	var assetIdExists bool
	err := db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)`,
		tableName,
		colName).Scan(&assetIdExists)
	if err != nil {
		return false, err
	}
	zap.S().Infof("Column %v exists in table %v: %v", colName, tableName, assetIdExists)
	return assetIdExists, nil
}

func DropTableConstraint(constraintName string, tableName string, db *sql.DB) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s", tableName, constraintName))
	return err
}

func DropColumn(colName, tableName string, db *sql.DB) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", tableName, colName))
	return err
}

func AddIntColumn(colName, tableName string, db *sql.DB) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s int", tableName, colName))
	return err
}

func AddFKConstraint(constraintName, tableName, colName, fkTableName, fkColName string, db *sql.DB) error {
	// skip if constraint already exists
	var constraintExists bool
	err := db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = $1)`,
		constraintName).Scan(&constraintExists)
	if err != nil {
		return err
	}
	if constraintExists {
		return nil
	}
	_, err = db.Exec(
		fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			tableName,
			constraintName,
			colName,
			fkTableName,
			fkColName))
	return err
}

func AddUniqueConstraint(constraintName, tableName string, colNames []string, db *sql.DB) error {
	// check if constraint already exists
	var constraintExists bool
	err := db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = $1)`,
		constraintName).Scan(&constraintExists)
	if err != nil {
		return err
	}
	if constraintExists {
		return nil
	}
	_, err = db.Exec(
		fmt.Sprintf(
			`ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s)`, tableName, constraintName, strings.Join(
				colNames,
				",")))
	return err
}
