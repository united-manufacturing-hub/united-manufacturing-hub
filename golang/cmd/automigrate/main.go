package main

import (
	"database/sql"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/automigrate/migrations"
	"go.uber.org/zap"
	"net/http"
	"os"
)

/*
	To add new migrations follow these steps:
	1. If the new release is a major or minor release, create a new folder in the migrations folder with the name of the new release.
	2. Then create a .go file in the new folder (or old folder if no major/minor) with the patch number of the new release.
	3. Inside create a function, which accept *sql.DB as argument and returns an error.
		- The function name must be V<MAJOR>x<MINOR>x<PATCH> (e.g. V0x10x0)
	4. Add the function to the migrationsList at the bottom of the migrations.go file.
	5. Done!
*/

var buildtime string

func setupLoggingMetricsHealthcheck() healthcheck.Handler {
	// Initialize zap logging
	logger.New("LOGGING_LEVEL")

	zap.S().Infof("This is auto-migrate build date: %s", buildtime)

	// pprof
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("localhost:1337", nil)
		if err != nil {
			zap.S().Errorf("Error starting pprof: %s", err)
		}
	}()

	// Prometheus
	metricsPath := "/metrics"
	metricsPort := ":2112"
	zap.S().Debugf("Setting up metrics %s %v", metricsPath, metricsPort)

	http.Handle(metricsPath, promhttp.Handler())
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe(metricsPort, nil)
		if err != nil {
			zap.S().Errorf("Error starting metrics: %s", err)
		}
	}()

	// Prometheus
	zap.S().Debugf("Setting up healthcheck")

	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()
	return health
}

func setupPostgres(health healthcheck.Handler) *sql.DB {
	// Postgres
	PQHost := os.Getenv("POSTGRES_HOST")
	PQPort := 5432
	PQUser := os.Getenv("POSTGRES_USER")
	PQPassword := os.Getenv("POSTGRES_PASSWORD")
	PWDBName := os.Getenv("POSTGRES_DATABASE")
	PQSSLMode := os.Getenv("POSTGRES_SSLMODE")
	if PQSSLMode == "" {
		PQSSLMode = "require"
	} else {
		zap.S().Warnf("Postgres SSL mode is set to %s", PQSSLMode)
	}

	return SetupDB(PQUser, PQPassword, PWDBName, PQHost, PQPort, health, PQSSLMode)
}

func main() {
	health := setupLoggingMetricsHealthcheck()
	db := setupPostgres(health)

	helmVersion, ok := migrations.StringToSemver(os.Getenv("VERSION"))
	if !ok {
		zap.S().Fatalf("VERSION is not a valid semver: %s", os.Getenv("VERSION"))
	}
	migrations.Migrate(helmVersion, db)

	ShutdownDB(db)
}
