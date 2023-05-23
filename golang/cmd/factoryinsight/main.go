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

package main

/*
Important principles: stateless as much as possible
*/

/*
Target architecture:

Incoming REST call --> helpers.go
There is one function for that specific call. It parses the parameters and executes further functions:
1. One or multiple function getting the data from the database (database.go)
2. Only one function processing everything. In this function no database calls are allowed to be as stateless as possible (dataprocessing.go)
Then the results are bundled together and a return JSON is created.
*/

import (
	"fmt"
	"github.com/gin-contrib/gzip"
	ginzap "github.com/gin-contrib/zap"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	apiV1 "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v1"
	v2controllers "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/controllers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/services"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

var (
	shutdownEnabled bool
)

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	PQHost, err := env.GetAsString("POSTGRES_HOST", false, "db")
	if err != nil {
		zap.S().Error(err)
	}

	PQPort, err := env.GetAsInt("POSTGRES_PORT", false, 5432)
	if err != nil {
		zap.S().Error(err)
	}

	// Read in other environment variables
	PQUser, err := env.GetAsString("POSTGRES_USER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	PQPassword, err := env.GetAsString("POSTGRES_PASSWORD", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	PQDBName, err := env.GetAsString("POSTGRES_DATABASE", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	// Loading up user accounts
	accounts := gin.Accounts{}

	zap.S().Debugf("Loading accounts from environment..")

	for i := 1; i <= 100; i++ {
		var tempUser, tempPassword string
		tempUser, err = env.GetAsString("CUSTOMER_NAME_"+strconv.Itoa(i), false, "")
		if err != nil {
			zap.S().Error(err)
		}
		tempPassword, err = env.GetAsString("CUSTOMER_PASSWORD_"+strconv.Itoa(i), false, "")
		if err != nil {
			zap.S().Error(err)
		}
		if err != nil {
			zap.S().Error(err)
		}
		if tempUser != "" && tempPassword != "" {
			zap.S().Infof("Added account for " + tempUser)
			accounts[tempUser] = tempPassword
		}
	}

	// also add admin access
	RESTUser, err := env.GetAsString("FACTORYINSIGHT_USER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	RESTPassword, err := env.GetAsString("FACTORYINSIGHT_PASSWORD", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	accounts[RESTUser] = RESTPassword

	// get version
	version, err := env.GetAsInt("VERSION", true, 2)
	if err != nil {
		zap.S().Fatal(err)
	}
	zap.S().Debugf("Starting program..")

	redisURI, err := env.GetAsString("REDIS_URI", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	redisPassword, err := env.GetAsString("REDIS_PASSWORD", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	redisDB := 0 // default database

	dryRun, err := env.GetAsBool("DRY_RUN", false, false)
	if err != nil {
		zap.S().Error(err)
	}
	internal.InitCache(redisURI, redisPassword, redisDB, dryRun)

	zap.S().Debugf("Cache initialized at %s", redisURI)

	health := healthcheck.NewHandler()
	shutdownEnabled = false
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(100))
	health.AddReadinessCheck("shutdownEnabled", isShutdownEnabled())
	go func() {
		/* #nosec G114 */
		errX := http.ListenAndServe("0.0.0.0:8086", health)
		if errX != nil {
			zap.S().Errorf("Error starting healthcheck: %s", errX)
		}
	}()

	zap.S().Debug("Healthcheck initialized at localhost:8086")

	sigs := make(chan os.Signal, 1)
	database.Connect(PQUser, PQPassword, PQDBName, PQHost, PQPort, sigs)

	zap.S().Debug("DB initialized")

	helpers.InsecureNoAuth, err = env.GetAsBool("INSECURE_NO_AUTH", false, false)
	if err != nil {
		zap.S().Error(err)
	}
	if helpers.InsecureNoAuth {
		for i := 0; i < 50; i++ {
			zap.S().Warnf("INSECURE_NO_AUTH is set to true. This is a security risk. Do not use in production.")
		}
		zap.S().Warnf("Sleeping for 10 seconds to allow you to cancel the program.")
		time.Sleep(10 * time.Second)
	}

	setupRestAPI(accounts, version)
	zap.S().Debug("REST API initialized")

	// Allow graceful shutdown
	signal.Notify(sigs, syscall.SIGTERM)

	go func() {
		// before you trapped SIGTERM your process would
		// have exited, so we are now on borrowed time.
		//
		// Kubernetes sends SIGTERM 30 seconds before
		// shutting down the pod.

		sig := <-sigs

		// Log the received signal
		zap.S().Infof("Received %v", sig)

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	select {} // block forever
}

func isShutdownEnabled() healthcheck.Check {
	return func() error {
		if shutdownEnabled {
			return fmt.Errorf("shutdown")
		}
		return nil
	}
}

// ShutdownApplicationGraceful shutsdown the entire application including MQTT and database
func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")
	shutdownEnabled = true

	time.Sleep(4 * time.Minute) // Wait until all remaining open connections are handled

	database.Shutdown()

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

// setupRestAPI initializes the REST API and starts listening
func setupRestAPI(accounts gin.Accounts, version int) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Add a ginzap middleware, which:
	//   - Logs all requests, like a combined access and error log.
	//   - Logs to stdout.
	//   - RFC3339 with UTC time format.
	router.Use(ginzap.Ginzap(zap.L(), time.RFC3339, true))

	// Logs all panic to error log
	//   - stack means whether output the stack info.
	router.Use(ginzap.RecoveryWithZap(zap.L(), true))

	// Use gzip for all requests
	router.Use(gzip.Gzip(gzip.DefaultCompression))

	// Healthcheck
	router.GET(
		"/", func(c *gin.Context) {
			c.String(http.StatusOK, "online")
		})

	// Version of the API
	if version >= 1 {
		zap.S().Infof("Starting API version 1")
		v1 := router.Group("/api/v1", gin.BasicAuth(accounts))
		if helpers.InsecureNoAuth {
			v1 = router.Group("/api/v1")
		}
		{
			// WARNING: Need to check in each specific handler whether the user is actually allowed to access it, so that valid user "ia" cannot access data for customer "abc"
			v1.GET("/:customer", apiV1.GetLocationsHandler)
			v1.GET("/:customer/:location", apiV1.GetAssetsHandler)
			v1.GET("/:customer/:location/:asset", apiV1.GetValuesHandler)
			v1.GET("/:customer/:location/:asset/:value", apiV1.GetDataHandler)
		}
	}

	if version >= 2 {
		zap.S().Infof("Starting API version 2")
		v2 := router.Group("/api/v2", gin.BasicAuth(accounts))
		if helpers.InsecureNoAuth {
			v2 = router.Group("/api/v2")
		}
		{
			v2.GET("/treeStructure", v2controllers.GetTreeStructureHandler)
			v2.GET("/:enterpriseName", v2controllers.GetSitesHandler)
			v2.GET("/:enterpriseName/configuration", v2controllers.GetConfigurationHandler)
			v2.GET("/:enterpriseName/database-stats", v2controllers.GetDatabaseStatisticsHandler)
			v2.GET("/:enterpriseName/:siteName", v2controllers.GetAreasHandler)
			v2.GET("/:enterpriseName/:siteName/:areaName", v2controllers.GetProductionLinesHandler)
			v2.GET("/:enterpriseName/:siteName/:areaName/:productionLineName", v2controllers.GetWorkCellsHandler)
			v2.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName",
				v2controllers.GetDataFormatHandler)
			v2.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/getValues",
				v2controllers.GetValueTree)
			v2.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tags",
				v2controllers.GetTagGroupsHandler)
			v2.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tags/:tagGroupName",
				v2controllers.GetTagsHandler)
			v2.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tags/:tagGroupName/:tagName",
				v2controllers.GetTagsDataHandler)
			v2.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/kpis",
				v2controllers.GetKpisMethodsHandler)
			v2.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/kpis/:kpisMethod",
				v2controllers.GetKpisDataHandler)
			v2.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tables",
				v2controllers.GetTableTypesHandler)
			v2.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tables/:tableType",
				v2controllers.GetTableDataHandler)
		}
		go services.TagPrefetch()
	}

	/*
		v3 := router.Group("/api/v3", gin.BasicAuth(accounts))
		if helpers.InsecureNoAuth {
			v2 = router.Group("/api/v3")
		}
		{
			// Get all sites for a given enterprise
			v3.GET("/:enterpriseName", v3controllers.GetSitesHandler)
			// Get all areas for a given site)
			v3.GET("/:enterpriseName/:siteName", v3controllers.GetAreasHandler)
			// Get all production lines for a given area
			v3.GET("/:enterpriseName/:siteName/:areaName", v3controllers.GetProductionLinesHandler)
			// Get all work cells for a given production line
			v3.GET("/:enterpriseName/:siteName/:areaName/:productionLineName", v3controllers.GetWorkCellsHandler)
			// Get all data format for a given work cell
			v3.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName",
				v3controllers.GetDataFormatHandler)
			// Get all tag groups for a given work cell
			v3.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tags",
				v3controllers.GetTagGroupsHandler)
			// Get all tags for a given tag group
			v3.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tags/:tagGroupName",
				v3controllers.GetTagsHandler)
			// Get specific data for a give work cell
			v3.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tags/:tagGroupName/:tagName",
				v3controllers.GetTagsDataHandler)
			// Get KPIs methods for a given work cell
			v3.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/kpis",
				v3controllers.GetKpisMethodsHandler)
			// Get specific KPI data for a given work cell
			v3.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/kpis/:kpisMethod",
				v3controllers.GetKpisDataHandler)
			// Get tables types for a given work cell
			v3.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tables",
				v3controllers.GetTableTypesHandler)
			// Get specific table data for a given work cell
			v3.GET(
				"/:enterpriseName/:siteName/:areaName/:productionLineName/:workCellName/tables/:tableType",
				v3controllers.GetTableDataHandler)

		}

	*/
	//dataFormat
	err := router.Run(":80")
	if err != nil {
		zap.S().Fatalf("Error starting the server: %s", err)
	}
}
