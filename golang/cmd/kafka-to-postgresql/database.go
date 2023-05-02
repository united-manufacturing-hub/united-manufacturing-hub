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

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/heptiolabs/healthcheck"
	_ "github.com/lib/pq"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"regexp"
	"strings"
	"time"
)

var db *sql.DB
var statement *StatementRegistry

var isDryRun bool

// SetupDB setups the db and stores the handler in a global variable in database.go
func SetupDB(
	PQUser string,
	PQPassword string,
	PWDBName string,
	PQHost string,
	PQPort int,
	health healthcheck.Handler,
	dryRun bool,
	sslmode string) {

	psqlInfo := fmt.Sprintf(
		"host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=%s",
		PQHost,
		PQPort,
		PQUser,
		PQPassword,
		PWDBName,
		sslmode)
	var err error
	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		zap.S().Fatalf("Failed to open database connection: %s", err)
	}

	if dryRun {
		zap.S().Infof("Running in DRY_RUN mode. ALl statements will be rolled back and printed automatically")
		isDryRun = true
	} else {
		isDryRun = false
	}
	var ok bool
	var perr error
	if ok, perr = IsPostgresSQLAvailable(); !ok {
		zap.S().Fatalf("Postgres not yet available: %s", perr)
	}

	db.SetMaxOpenConns(20)

	// Healthcheck
	health.AddReadinessCheck("database", healthcheck.DatabasePingCheck(db, internal.OneSecond))

	health.AddLivenessCheck("database", healthcheck.DatabasePingCheck(db, 30*time.Second))

	statement = NewStatementRegistry()
}

// IsPostgresSQLAvailable returns if the database is reachable by PING command
func IsPostgresSQLAvailable() (bool, error) {
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
func ShutdownDB() {

	zap.S().Infof("Closing statement registry")
	statement.Shutdown()

	zap.S().Infof("Closing database connection")

	if err := db.Close(); err != nil {
		zap.S().Fatalf("Failed to close database connection: %s", err)
	}
}

// RecoveryType Enum used to identify which operation to perform, when the db returns an error
type RecoveryType int32

const (
	Other        RecoveryType = 0
	DatabaseDown RecoveryType = 1
	DiscardValue RecoveryType = 2
)

// GetPostgresErrorRecoveryOptions checks if the error is recoverable
func GetPostgresErrorRecoveryOptions(err error) RecoveryType {
	if err == nil {
		return Other
	}

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

	var matchedOOFRegex = regexp.MustCompile(`pq: value "-*\d+" is out of range for type integer`)
	var matchTsOORRegex = regexp.MustCompile(`pq: timestamp out of range: .+`)

	matchedOutOfRange := matchedOOFRegex.MatchString(errorString)
	matchedTsOutOfRange := matchTsOORRegex.MatchString(errorString)

	isRecoverableByDiscarding := matchedOutOfRange || matchedTsOutOfRange
	if isRecoverableByDiscarding {
		return DiscardValue
	}
	return Other
}

// GetAssetTableID gets the assetID from the database
func GetAssetTableID(customerID string, location string, assetID string) (AssetTableID uint32, success bool) {

	success = false
	// Get from cache if possible
	var cacheHit bool
	AssetTableID, cacheHit = GetCacheAssetTableId(customerID, location, assetID)
	if cacheHit {
		success = true
		return
	}

	err := statement.SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId.QueryRow(
		assetID,
		location,
		customerID).Scan(&AssetTableID)
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Debugf(
			"[GetAssetTableID] No Results Found for assetID: %s, location: %s, customerID: %s",
			assetID,
			location,
			customerID)
		// This can potentially lead to race conditions, if another thread adds the same asset too
		err = AddAsset(assetID, location, customerID)
		if err != nil {
			zap.S().Errorf("Failed to add new Asset: %s", err.Error())
			return 0, false
		} else {
			return GetAssetTableID(customerID, location, assetID)
		}
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

// GetComponentID gets the componentID from the database
func GetComponentID(assetID uint32, componentName string) (componentID int32, success bool) {
	zap.S().Debugf("[GetComponentID] assetID: %d, componentName: %s", assetID, componentName)
	success = false
	err := statement.SelectIdFromComponentTableByAssetIdAndComponentName.QueryRow(
		assetID,
		componentName).Scan(&componentID)
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Errorf("No Results Found assetID: %d, componentName: %s", assetID, componentName)

		return
	} else if err != nil {
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
	success = true

	return
}

// AddAsset adds an asset to the database
func AddAsset(assetID string, location string, customerID string) (err error) {
	var txn *sql.Tx
	txn, err = db.Begin()
	if err != nil {
		return err
	}

	stmt := txn.Stmt(statement.InsertIntoAssetTable)

	_, err = stmt.Exec(assetID, location, customerID)
	if err != nil {
		return err
	}

	err = txn.Commit()
	if err != nil {
		return err
	}

	return err
}

// GetProductTableId gets the productID from the database using the productname and AssetTableId
func GetProductTableId(productName string, AssetTableId uint32) (ProductTableId uint32, success bool) {
	success = false
	// Get from cache if possible
	var cacheHit bool
	ProductTableId, cacheHit = GetCacheProductTableId(productName, AssetTableId)
	if cacheHit {
		success = true
		return
	}

	err := statement.SelectProductIdFromProductTableByAssetIdAndProductName.QueryRow(
		AssetTableId,
		productName).Scan(&ProductTableId)
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Debugf(
			"[GetProductTableId] No Results Found for productName: %s, AssetTableId: %d",
			productName,
			AssetTableId)
		return 0, false
	} else if err != nil {
		zap.S().Debugf("[GetProductTableId] Error: %s", err)
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

	go PutCacheProductTableId(productName, AssetTableId, ProductTableId)
	zap.S().Debugf("Stored ProductName to cache")

	success = true
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

// GetUniqueProductID gets the unique productID from the database using the UniqueProductAlternativeID and AssetTableId
func GetUniqueProductID(UniqueProductAlternativeId string, AssetTableId uint32) (
	UniqueProductTableId uint32,
	success bool) {
	success = false

	// Get from cache if possible
	var cacheHit bool
	UniqueProductTableId, cacheHit = GetCacheUniqueProductTableId(UniqueProductAlternativeId, AssetTableId)
	if cacheHit {
		success = true
		return
	}

	err := statement.SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndAssetIdOrderedByTimeStampDesc.QueryRow(
		UniqueProductAlternativeId,
		AssetTableId).Scan(&UniqueProductTableId)
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Debugf(
			"[GetUniqueProductID] No Results Found for UniqueProductAlternativeId: %s, AssetTableId: %d",
			UniqueProductAlternativeId,
			AssetTableId)

		return 0, false
	} else if err != nil {
		zap.S().Debugf("[GetUniqueProductID] Error: %s", err)
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
	go PutCacheUniqueProductTableId(UniqueProductAlternativeId, AssetTableId, UniqueProductTableId)
	zap.S().Debugf("Stored ProductName to cache")

	success = true
	return
}

// GetLatestParentUniqueProductID gets the latest parent unique productID from the database using the UniqueProductAlternativeID and AssetTableId
func GetLatestParentUniqueProductID(ParentID string, DBAssetID uint32) (
	LatestparentUniqueProductId uint32,
	success bool) {
	success = false

	// Get from cache if possible
	var cacheHit bool
	LatestparentUniqueProductId, cacheHit = GetCacheLatestParentUniqueProductID(ParentID, DBAssetID)
	if cacheHit {
		success = true
		return
	}

	err := statement.SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndNotAssetId.QueryRow(
		ParentID,
		DBAssetID).Scan(&LatestparentUniqueProductId)
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Debugf("[GetUniqueProductID] No Results Found for ParentID: %s, DBAssetID: %d", ParentID, DBAssetID)

		return 0, false
	} else if err != nil {
		zap.S().Debugf("[GetUniqueProductID] Error: %s", err)
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

	go PutCacheLatestParentUniqueProductID(ParentID, DBAssetID, LatestparentUniqueProductId)
	zap.S().Debugf("Stored ProductName to cache")

	success = true
	return
}
