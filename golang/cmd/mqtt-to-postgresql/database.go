package main

import (
	"database/sql"
	"fmt"
	"github.com/beeker1121/goque"
	"github.com/heptiolabs/healthcheck"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/omeid/pgerror"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
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
	for {
		var ok bool
		var perr error
		if ok, perr = IsPostgresSQLAvailable(); ok {
			break
		}
		zap.S().Warnf("Postgres not yet available: %s", perr)
		time.Sleep(1 * time.Second)
	}

	db.SetMaxOpenConns(20)
	// Healthcheck
	health.AddReadinessCheck("database", healthcheck.DatabasePingCheck(db, 1*time.Second))

	statement = newStatementRegistry()
}

//ValidatePGInt validates that input is smaller than pg max int
func ValidatePGInt(input uint32) bool {
	return input < 2147483647 //https://www.postgresql.org/docs/current/datatype-numeric.html
}

//ValidateStruct iterates structs fields, checking all Uint32 to be inside the postgres int limit
func ValidateStruct(vstruct interface{}) bool {
	v := reflect.ValueOf(vstruct)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Kind() == reflect.Uint32 {
			if !ValidatePGInt(uint32(field.Uint())) {
				return false
			}
		}
	}
	return true
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

// PGErrorHandlingTransaction logs and handles postgresql errors in transactions
func PGErrorHandlingTransaction(sqlStatement string, err error, txn *sql.Tx) (returnedErr error) {

	PGErrorHandling(sqlStatement, err)

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
		PGErrorHandling("txn.Rollback()", err2)
	}
	returnedErr = err

	return
}

// PGErrorHandling logs and handles postgresql errors
func PGErrorHandling(sqlStatement string, err error) {

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

// CommitOrRollbackOnError runs at the end of every database function
// Either commits or rolls back the transaction, depending on if the tx was successful
func CommitOrRollbackOnError(txn *sql.Tx, errIn error) (errOut error) {
	if txn == nil {
		PGErrorHandling("Transaction is nil", errIn)
		return
	}

	if errIn != nil {
		zap.S().Debugf("Got error from callee: %s", errIn)
		debug.PrintStack()
		errOut = txn.Rollback()
		return
	}

	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		errOut = txn.Rollback()
		if errOut != nil {
			if errOut != sql.ErrTxDone {
				PGErrorHandling("txn.Rollback()", errOut)
			} else {
				zap.S().Warnf("%s", errOut)
			}

		}
	} else {
		errOut = txn.Commit()
		if errOut != nil {
			if errOut != sql.ErrTxDone {
				PGErrorHandling("txn.Commit()", errOut)
			} else {
				zap.S().Warnf("Commit failed: %s", errOut)
			}
		}
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

// GetAssetID gets the assetID from the database
func GetAssetID(customerID string, location string, assetID string, recursionDepth int64) (DBassetID uint32, success bool) {
	zap.S().Debugf("[GetAssetID] customerID: %s, location: %s, assetID: %s", customerID, location, assetID)

	success = false
	// Get from cache if possible
	var cacheHit bool
	DBassetID, cacheHit = internal.GetAssetIDFromCache(customerID, location, assetID)
	if cacheHit { // data found

		success = true
		return
	}

	err := statement.SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId.QueryRow(assetID, location, customerID).Scan(&DBassetID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found for assetID: %s, location: %s, customerID: %s", assetID, location, customerID)
	} else if err != nil {
		switch GetPostgresErrorRecoveryOptions(err) {
		case Unrecoverable:
			PGErrorHandling("GetAssetID db.QueryRow()", err)
		case TryAgain:
			internal.SleepBackedOff(recursionDepth, 10*time.Millisecond, 1*time.Second)
			return GetAssetID(customerID, location, assetID, recursionDepth+1)
		case DiscardValue:
			return 0, false

		}
		return
	}

	// Store to cache if not yet existing
	go internal.StoreAssetIDToCache(customerID, location, assetID, DBassetID)
	zap.S().Debugf("Stored AssetID to cache")

	success = true
	return
}

// GetProductID gets the productID for a asset and a productName from the database
func GetProductID(DBassetID uint32, productName string, recursionDepth int64) (productID int32, err error, success bool) {
	zap.S().Debugf("[GetProductID] DBassetID: %d, productName: %s", DBassetID, productName)
	success = false

	err = statement.SelectProductIdFromProductTableByAssetIdAndProductName.QueryRow(DBassetID, productName).Scan(&productID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found DBAssetID: %d, productName: %s", DBassetID, productName)
		return
	} else if err != nil {
		switch GetPostgresErrorRecoveryOptions(err) {
		case Unrecoverable:
			PGErrorHandling("GetProductID db.QueryRow()", err)
		case TryAgain:
			internal.SleepBackedOff(recursionDepth, 10*time.Millisecond, 1*time.Second)
			return GetProductID(DBassetID, productName, recursionDepth+1)
		case DiscardValue:
			return
		}
		return
	}
	success = true

	return
}

// GetComponentID gets the componentID from the database
func GetComponentID(assetID uint32, componentName string, recursionDepth int64) (componentID int32, success bool) {
	zap.S().Debugf("[GetComponentID] assetID: %d, componentName: %s", assetID, componentName)
	success = false
	err := statement.SelectIdFromComponentTableByAssetIdAndComponentName.QueryRow(assetID, componentName).Scan(&componentID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found assetID: %d, componentName: %s", assetID, componentName)

		return
	} else if err != nil {
		switch GetPostgresErrorRecoveryOptions(err) {
		case Unrecoverable:
			PGErrorHandling("GetComponentID() db.QueryRow()", err)
		case TryAgain:
			internal.SleepBackedOff(recursionDepth, 10*time.Millisecond, 1*time.Second)
			return GetComponentID(assetID, componentName, recursionDepth+1)
		case DiscardValue:
			return 0, false
		}
		return
	}
	success = true

	return
}

func GetUniqueProductID(aid string, DBassetID uint32, recursionDepth int64) (uid uint32, err error, success bool) {
	zap.S().Debugf("[GetUniqueProductID] aid: %s, DBassetID: %d", aid, DBassetID)
	success = false
	err = statement.SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndAssetIdOrderedByTimeStampDesc.QueryRow(aid, DBassetID).Scan(&uid)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found aid: %s, DBassetID: %d", aid, DBassetID)

		return
	} else if err != nil {
		switch GetPostgresErrorRecoveryOptions(err) {
		case Unrecoverable:
			PGErrorHandling("GetUniqueProductID db.QueryRow()", err)
		case TryAgain:
			internal.SleepBackedOff(recursionDepth, 10*time.Millisecond, 1*time.Second)
			GetUniqueProductID(aid, DBassetID, recursionDepth+1)
		case DiscardValue:
			return 0, err, false
		}
		return
	}
	success = true

	return
}

func GetLatestParentUniqueProductID(aid string, assetID uint32) (uid int32, success bool) {
	zap.S().Debugf("[GetLatestParentUniqueProductID] aid: %s, assetID: %d", aid, assetID)
	success = false
	err := statement.SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndNotAssetId.QueryRow(aid, assetID).Scan(&uid)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found aid: %s, assetID: %d", aid, assetID)

		return
	} else if err != nil {
		switch GetPostgresErrorRecoveryOptions(err) {
		case Unrecoverable:
			PGErrorHandling("GetLatestParentUniqueProductID db.QueryRow()", err)
		case TryAgain:
			return GetLatestParentUniqueProductID(aid, assetID)
		case DiscardValue:
			return 0, false
		}
		return
	}
	success = true

	return
}

func CheckIfProductExists(productId int32, DBassetID uint32) (exists bool, err error) {
	_, cacheHit := internal.GetProductIDFromCache(productId, DBassetID)
	if cacheHit {
		return true, nil
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		return false, err
	}

	var rowCount int32

	stmt := txn.Stmt(statement.SelectProductExists)
	err = stmt.QueryRow(productId).Scan(&rowCount)
	if err != nil {
		zap.S().Debugf("Failed to scan rows ", err)
		return false, err
	}

	err = txn.Commit()
	if err != nil {
		return false, err
	}

	return rowCount == 1, err
}

// AddAssetIfNotExisting adds an asset to the db if it is not existing yet
func AddAssetIfNotExisting(assetID string, location string, customerID string, recursionDepth int) (err error) {

	// Get from cache if possible
	var cacheHit bool
	_, cacheHit = internal.GetAssetIDFromCache(customerID, location, assetID)
	if cacheHit { // data found

		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		switch GetPostgresErrorRecoveryOptions(err) {
		case Unrecoverable:
			ShutdownApplicationGraceful()
		case TryAgain:
			if recursionDepth < 10 {
				internal.SleepBackedOff(int64(recursionDepth), 10*time.Millisecond, 1*time.Second)
				err = nil
				err = AddAssetIfNotExisting(assetID, location, customerID, recursionDepth+1)
			} else {
				return err
			}
		case DiscardValue:
			return err
		}
	}

	zap.S().Debugf("txn: ", txn, err)

	defer func() {
		errx := CommitOrRollbackOnError(txn, err)
		if errx != nil {
			err = errx
			return
		}
	}()

	stmt := txn.Stmt(statement.InsertIntoAssetTable)
	zap.S().Debugf("stmt: ", stmt)

	_, err = stmt.Exec(assetID, location, customerID)
	zap.S().Debugf("Exec: ", err)
	if err != nil {
		switch GetPostgresErrorRecoveryOptions(err) {
		case Unrecoverable:
			ShutdownApplicationGraceful()
		case TryAgain:
			if recursionDepth < 10 {
				internal.SleepBackedOff(int64(recursionDepth), 10*time.Millisecond, 1*time.Second)
				err = nil
				err = AddAssetIfNotExisting(assetID, location, customerID, recursionDepth+1)
			} else {
				return
			}
		case DiscardValue:
			return
		}
	}

	return nil
}

type RecoveryType int32

const (
	Unrecoverable RecoveryType = 0
	TryAgain      RecoveryType = 1
	DiscardValue  RecoveryType = 2
)

//GetPostgresErrorRecoveryOptions checks if the error is recoverable
func GetPostgresErrorRecoveryOptions(err error) RecoveryType {

	// Why go allows returning errors, that are not exported is beyond me
	errorString := err.Error()
	isRecoverableByRetrying := strings.Contains(errorString, "sql: database is closed") ||
		strings.Contains(errorString, "driver: bad connection") ||
		strings.Contains(errorString, "connect: connection refused") ||
		strings.Contains(errorString, "pq: the database system is shutting down") ||
		strings.Contains(errorString, "connect: no route to host")
	if isRecoverableByRetrying {
		time.Sleep(1 * time.Second)
		return TryAgain
	}

	matchedOutOfRange, err := regexp.MatchString(`pq: value "-*\d+" is out of range for type integer`, errorString)

	isRecoverableByDiscarding := matchedOutOfRange
	if isRecoverableByDiscarding {
		return DiscardValue
	}

	zap.S().Errorf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	zap.S().Errorf("!! RUN INTO NON RECOVERABLE ERROR !!")
	zap.S().Errorf("%s", err.Error())
	zap.S().Errorf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")

	return Unrecoverable
}

func storeItemsIntoDatabaseRecommendation(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoRecommendationTable)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt recommendationStruct
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.UID, pt.RecommendationType, pt.Enabled, pt.RecommendationValues, pt.RecommendationTextEN, pt.RecommendationTextDE, pt.DiagnoseTextEN, pt.DiagnoseTextDE)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue
		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseRecommendation, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err

}

//goland:noinspection SqlResolve
func storeItemsIntoDatabaseProcessValueFloat64(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	defer func() {
		errx := CommitOrRollbackOnError(txn, err)
		if errx != nil {
			err = errx

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
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(pq.CopyIn("tmp_processvaluetable64", "timestamp", "asset_id", "value", "valuename"))
		if err != nil {
			faultyItems = items

			return
		}

		for _, item := range items {
			var pt processValueQueueF64
			err = item.ToObjectFromJSON(&pt)
			if err != nil {
				zap.S().Errorf("Failed to unmarshal item", item)
				continue
			}

			timestamp := time.Unix(0, int64(pt.TimestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")

			_, err = stmt.Exec(timestamp, pt.DBAssetID, pt.ValueFloat64, pt.Name)
			if err != nil {
				faultyItems = append(faultyItems, item)

				err = nil
				continue

			}
		}
		err = stmt.Close()
		if err != nil {
			faultyItems = items

			return
		}

	}

	// 3. Prepare statement: copy from temp table into main table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			INSERT INTO processvaluetable (SELECT * FROM tmp_processvaluetable64) ON CONFLICT DO NOTHING;
		`)
		if err != nil {
			faultyItems = items

			return
		}

		_, err = stmt.Exec()
		if err != nil {
			faultyItems = items

			return
		}

		err = stmt.Close()
		if err != nil {
			faultyItems = items

			return
		}
	}

	return
}

//goland:noinspection SqlResolve
func storeItemsIntoDatabaseProcessValueString(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	defer func() {
		errx := CommitOrRollbackOnError(txn, err)
		if errx != nil {
			err = errx

			return
		}
	}()

	// 1. Prepare statement: create temp table
	//These statements' auto close
	{
		stmt := txn.Stmt(statement.CreateTmpProcessValueTableString)
		_, err = stmt.Exec()
		if err != nil {
			faultyItems = items

			return
		}
	}
	// 2. Prepare statement: copying into temp table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(pq.CopyIn("tmp_processvaluestringtable", "timestamp", "asset_id", "value", "valuename"))
		if err != nil {
			faultyItems = items

			return
		}

		var workingItems []*goque.PriorityItem
		for _, item := range items {
			var pt processValueStringQueue
			err = item.ToObjectFromJSON(&pt)
			if err != nil {
				zap.S().Errorf("Failed to unmarshal item", item)
				continue
			}

			timestamp := time.Unix(0, int64(pt.TimestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")

			_, err = stmt.Exec(timestamp, pt.DBAssetID, pt.Value, pt.Name)
			if err != nil {
				faultyItems = append(faultyItems, item)

				err = nil
				continue

			} else {
				workingItems = append(workingItems, item)
			}
		}

		if err != nil {

		}

		err = stmt.Close()
		if err != nil {
			faultyItems = items

			return
		}

	}

	// 3. Prepare statement: copy from temp table into main table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			INSERT INTO processvaluestringtable (SELECT * FROM tmp_processvaluestringtable) ON CONFLICT DO NOTHING;
		`)
		if err != nil {
			faultyItems = items

			return
		}

		_, err = stmt.Exec()
		if err != nil {
			faultyItems = items

			return
		}

		err = stmt.Close()
		if err != nil {
			faultyItems = items

			return
		}
	}

	return
}

//goland:noinspection SqlResolve,SqlResolve
func storeItemsIntoDatabaseProcessValue(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	defer func() {
		errx := CommitOrRollbackOnError(txn, err)
		if errx != nil {
			err = errx

			return
		}
	}()

	// 1. Prepare statement: create temp table
	//These statements' auto close
	{
		stmt := txn.Stmt(statement.CreateTmpProcessValueTable)
		_, err = stmt.Exec()
		if err != nil {
			faultyItems = items

			return
		}
	}
	// 2. Prepare statement: copying into temp table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(pq.CopyIn("tmp_processvaluetable", "timestamp", "asset_id", "value", "valuename"))
		if err != nil {
			faultyItems = items

			return
		}
		for _, item := range items {
			var pt processValueQueueI32
			err = item.ToObjectFromJSON(&pt)
			if err != nil {
				zap.S().Errorf("Failed to unmarshal item", item)
				continue
			}

			timestamp := time.Unix(0, int64(pt.TimestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")

			_, err = stmt.Exec(timestamp, pt.DBAssetID, pt.ValueInt32, pt.Name)
			if err != nil {
				faultyItems = append(faultyItems, item)

				err = nil
				continue

			}
		}

		err = stmt.Close()
		if err != nil {
			faultyItems = items

			return
		}
	}

	// 3. Prepare statement: copy from temp table into main table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			INSERT INTO processvaluetable (SELECT * FROM tmp_processvaluetable) ON CONFLICT DO NOTHING;
		`)
		if err != nil {
			faultyItems = items

			return
		}

		_, err = stmt.Exec()
		if err != nil {
			faultyItems = items

			return
		}

		err = stmt.Close()
		if err != nil {
			faultyItems = items

			return
		}
	}

	return
}

//goland:noinspection SqlResolve
func storeItemsIntoDatabaseCount(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	defer func() {
		errx := CommitOrRollbackOnError(txn, err)
		if errx != nil {
			err = errx

			return
		}
	}()

	// 1. Prepare statement: create temp table
	//These statements' auto close
	{
		stmt := txn.Stmt(statement.CreateTmpCountTable)
		_, err = stmt.Exec()
		if err != nil {
			faultyItems = items

			return
		}
	}
	// 2. Prepare statement: copying into temp table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(pq.CopyIn("tmp_counttable", "timestamp", "asset_id", "count", "scrap"))
		if err != nil {
			faultyItems = items

			return
		}

		for _, item := range items {
			var pt countQueue
			err = item.ToObjectFromJSON(&pt)
			if err != nil {
				zap.S().Errorf("Failed to unmarshal item", item)
				continue
			}

			timestamp := time.Unix(0, int64(pt.TimestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")

			_, err = stmt.Exec(timestamp, pt.DBAssetID, pt.Count, pt.Scrap)
			if err != nil {
				faultyItems = append(faultyItems, item)

				err = nil
				continue

			}
		}

		err = stmt.Close()
		if err != nil {
			faultyItems = items

			return
		}

	}

	// 3. Prepare statement: copy from temp table into main table
	{
		var stmt *sql.Stmt
		stmt, err = txn.Prepare(`
			INSERT INTO counttable (SELECT * FROM tmp_counttable) ON CONFLICT DO NOTHING;
		`)
		if err != nil {
			faultyItems = items

			return
		}

		_, err = stmt.Exec()
		if err != nil {
			faultyItems = items

			return
		}

		err = stmt.Close()
		if err != nil {
			faultyItems = items

			return
		}
	}

	return
}

func storeItemsIntoDatabaseState(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items
		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoStateTable)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt stateQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.DBAssetID, pt.State)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue
		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseState, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func storeItemsIntoDatabaseScrapCount(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.UpdateCountTableScrap)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt scrapCountQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.DBAssetID, pt.Scrap)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseScrapCount, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

//CommitWorking commits the transaction if there are no errors with the processable items. Otherwise, it will just try to process the items, that haven't failed.
func CommitWorking(items []*goque.PriorityItem, faultyItems []*goque.PriorityItem, txn *sql.Tx, workingItems []*goque.PriorityItem, fnc func(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error), recursionDepth int) ([]*goque.PriorityItem, []*goque.PriorityItem, error) {
	zap.S().Debugf("CommitWorking len: %i, faultylen: %i, workingItems: %i, depth: %i for %s", len(items), len(faultyItems), len(workingItems), recursionDepth, GetFunctionName(fnc))
	var errx error
	if len(faultyItems) > 0 {
		errx = txn.Rollback()
		if errx != nil && GetPostgresErrorRecoveryOptions(errx) == Unrecoverable {
			zap.S().Errorf("Failed to rollback tx")
			return nil, items, errx
		}
		var innerFaultyItems []*goque.PriorityItem
		innerFaultyItems, errx = fnc(workingItems, recursionDepth)
		if errx != nil {
			faultyItems = append(faultyItems, innerFaultyItems...)
			return nil, faultyItems, errx
		}
	} else {
		if isDryRun {
			zap.S().Debugf("PREPARED STATEMENT")
			errx = txn.Rollback()
			if errx != nil {
				if errx != sql.ErrTxDone {
					PGErrorHandling("txn.Rollback()", errx)
				} else {
					zap.S().Warnf("%s", errx)
				}

			}
		} else {
			errx = txn.Commit()
			if errx != nil {
				if errx != sql.ErrTxDone {
					switch GetPostgresErrorRecoveryOptions(errx) {
					case Unrecoverable:
						ShutdownApplicationGraceful()
					case TryAgain:
						if recursionDepth < 10 {
							internal.SleepBackedOff(int64(recursionDepth), 10*time.Millisecond, 1*time.Second)
							errx = nil
							faultyItems, faultyItems, errx = CommitWorking(items, faultyItems, txn, workingItems, fnc, recursionDepth+1)
						}
					case DiscardValue:
						//This shouldn't happen here !
						ShutdownApplicationGraceful()
					}

				} else {
					zap.S().Warnf("Commit failed: %s", errx)
				}
			} else {
				zap.S().Debugf("Commited %i items with fnc: ", len(workingItems), GetFunctionName(fnc))
			}
		}
	}
	return faultyItems, nil, nil
}

func storeItemsIntoDatabaseUniqueProduct(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoUniqueProductTable)
	var workingItems []*goque.PriorityItem
	var missingItems []*goque.PriorityItem

	for _, item := range items {
		var pt uniqueProductQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		var productExists bool
		productExists, err = CheckIfProductExists(pt.ProductID, pt.DBAssetID)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			return
		}
		if productExists {
			// Create statement
			_, err = stmt.Exec(pt.DBAssetID, pt.BeginTimestampMs, NewNullInt64(int64(pt.EndTimestampMs)), pt.ProductID, pt.IsScrap, pt.UniqueProductAlternativeID)
			if err != nil {
				faultyItems = append(faultyItems, item)

				err = nil
				continue

			} else {
				workingItems = append(workingItems, item)
			}
		} else {
			zap.S().Debugf("Product %d does not yet exist", pt.ProductID)
			missingItems = append(missingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseUniqueProduct, recursionDepth)
	if err2 != nil {
		innerFaultyItems = append(innerFaultyItems, missingItems...)
		return innerFaultyItems, err2
	}

	faultyItems = append(faultyItems, missingItems...)
	return faultyItems, err

}

func storeItemsIntoDatabaseProductTag(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoProductTagTable)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt productTagQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		var uid uint32
		var success bool
		uid, err, success = GetUniqueProductID(pt.AID, pt.DBAssetID, 0)
		if err != nil || !success {
			zap.S().Errorf("Stopped writing productTag in Database, uid not found. AID: %s, DBAssetID %d", pt.AID, pt.DBAssetID)
			faultyItems = append(faultyItems, item)
			err = nil
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.Name, pt.Value, pt.TimestampMs, uid)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue
		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseProductTag, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func storeItemsIntoDatabaseProductTagString(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}
	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoProductTagStringTable)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt productTagStringQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		var uid uint32
		var success bool
		uid, err, success = GetUniqueProductID(pt.AID, pt.DBAssetID, 0)
		if err != nil || !success {
			zap.S().Errorf("Stopped writing productTag in Database, uid not found. AID: %s, DBAssetID %d", pt.AID, pt.DBAssetID)
			faultyItems = append(faultyItems, item)
			err = nil
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.Name, pt.Value, pt.TimestampMs, uid)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseProductTagString, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func storeItemsIntoDatabaseAddParentToChild(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoProductInheritanceTable)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt addParentToChildQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		var childUid uint32
		var success bool
		childUid, err, success = GetUniqueProductID(pt.ChildAID, pt.DBAssetID, 0)
		if err != nil || !success {
			zap.S().Errorf("Stopped writing addParentToChild in Database, childUid not found. ChildAID: %s, DBAssetID: %d", pt.ChildAID, pt.DBAssetID)
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		}
		var parentUid int32
		parentUid, success = GetLatestParentUniqueProductID(pt.ParentAID, pt.DBAssetID)
		if !success {
			zap.S().Errorf("Stopped writing addParentToChild in Database, parentUid not found. ChildAID: %s, DBAssetID: %d", pt.ChildAID, pt.DBAssetID)
			faultyItems = append(faultyItems, item)
			err = nil
			continue
		}
		// Create statement
		zap.S().Debugf("[storeItemsIntoDatabaseAddParentToChild] ParentUID: %d, childUID: %d", parentUid, childUid)
		_, err = stmt.Exec(parentUid, childUid, pt.TimestampMs)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseAddParentToChild, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func storeItemsIntoDatabaseShift(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoShiftTable)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt addShiftQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.TimestampMsEnd, pt.DBAssetID, 1) //type is always 1 for now (0 would be no shift)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseShift, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func storeItemsIntoDatabaseUniqueProductScrap(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.UpdateUniqueProductTableSetIsScrap)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt scrapUniqueProductQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.UID, pt.DBAssetID)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseUniqueProductScrap, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func storeItemsIntoDatabaseAddProduct(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		zap.S().Errorf("Failed to open txn: %s", err)
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoProductTable)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt addProductQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.DBAssetID, pt.ProductName, pt.TimePerUnitInSeconds)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseAddProduct, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func storeItemsIntoDatabaseAddOrder(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoOrderTable)
	var workingItems []*goque.PriorityItem
	var missingItems []*goque.PriorityItem

	for _, item := range items {
		var pt addOrderQueue

		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		var productExists bool
		productExists, err = CheckIfProductExists(pt.ProductID, pt.DBAssetID)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			return
		}
		if productExists {
			// Create statement
			_, err = stmt.Exec(pt.OrderName, pt.ProductID, pt.TargetUnits, pt.DBAssetID)
			if err != nil {
				faultyItems = append(faultyItems, item)

				err = nil
				continue
			} else {
				workingItems = append(workingItems, item)
			}
		} else {
			zap.S().Debugf("Product %d does not yet exist", pt.ProductID)
			missingItems = append(missingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseUniqueProduct, recursionDepth)
	if err2 != nil {
		innerFaultyItems = append(innerFaultyItems, missingItems...)
		return innerFaultyItems, err2
	}

	faultyItems = append(faultyItems, missingItems...)
	return faultyItems, err
}

func storeItemsIntoDatabaseStartOrder(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.UpdateOrderTableSetBeginTimestamp)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt startOrderQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.OrderName, pt.DBAssetID)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseStartOrder, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func storeItemsIntoDatabaseEndOrder(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.UpdateOrderTableSetEndTimestamp)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt endOrderQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.TimestampMs, pt.OrderName, pt.DBAssetID)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseEndOrder, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func storeItemsIntoDatabaseAddMaintenanceActivity(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.InsertIntoMaintenanceActivities)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt addMaintenanceActivityQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.ComponentID, pt.Activity, pt.TimestampMs)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, storeItemsIntoDatabaseAddMaintenanceActivity, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func modifyStateInDatabase(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	defer func() {
		errx := CommitOrRollbackOnError(txn, err)
		if errx != nil {
			err = errx

			return
		}
	}()

	//These statements' auto close
	StmtGetLastInRange := txn.Stmt(statement.SelectLastStateFromStateTableInRange)
	StmtDeleteInRange := txn.Stmt(statement.DeleteFromStateTableByTimestampRangeAndAssetId)
	StmtInsertNewState := txn.Stmt(statement.InsertIntoStateTable)
	StmtDeleteOldState := txn.Stmt(statement.DeleteFromStateTableByTimestamp)

	for _, item := range items {
		var pt modifyStateQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		var val *sql.Rows
		val, err = StmtGetLastInRange.Query(pt.StartTimeStampMs, pt.DBAssetID)
		if err != nil {
			faultyItems = append(faultyItems, item)
			//DONT RESET ERROR HERE
			continue
		}

		if val.Next() {
			var (
				LastRowTimestamp    float64
				LastRowTimestampInt int64
				LastRowAssetId      int64
				LastRowState        int64
			)
			err = val.Scan(&LastRowTimestamp, &LastRowAssetId, &LastRowState)
			if err != nil {
				err = PGErrorHandlingTransaction("rows.Scan()", err, txn)
				if err != nil {
					errx := txn.Rollback()
					if errx != nil {
						switch GetPostgresErrorRecoveryOptions(errx) {
						case Unrecoverable:
							ShutdownApplicationGraceful()
						case TryAgain:
							// If unable to rollback tx return everything as faulty
							return items, errx
						case DiscardValue:
							// This shouldn't be possible here
							ShutdownApplicationGraceful()
						}
					}
					return
				}
			}
			LastRowTimestampInt = int64(int(LastRowTimestamp))

			err = val.Close()
			if err != nil {
				err = PGErrorHandlingTransaction("val.Close()", err, txn)
				if err != nil {
					switch GetPostgresErrorRecoveryOptions(err) {
					case Unrecoverable:
						ShutdownApplicationGraceful()
					case TryAgain:
					// This means that we were able to scan all rows, but the database disappeared before we could close the scanner, which is fine for us
					case DiscardValue:
						// This should not occur here
					}
					return
				}
			}

			_, err = StmtDeleteInRange.Exec(pt.StartTimeStampMs, pt.EndTimeStampMs, pt.DBAssetID)
			if err != nil {
				faultyItems = append(faultyItems, item)
				//DONT RESET ERROR HERE
				continue
			}

			_, err = StmtInsertNewState.Exec(pt.StartTimeStampMs, pt.DBAssetID, pt.NewState)
			if err != nil {
				faultyItems = append(faultyItems, item)
				//DONT RESET ERROR HERE
				continue
			}

			_, err = StmtDeleteOldState.Exec(LastRowTimestampInt)
			if err != nil {
				faultyItems = append(faultyItems, item)
				//DONT RESET ERROR HERE
				continue
			}

			_, err = StmtInsertNewState.Exec(pt.EndTimeStampMs, pt.DBAssetID, LastRowState)
			if err != nil {
				faultyItems = append(faultyItems, item)
				//DONT RESET ERROR HERE
				continue
			}

		}
	}

	return
}

func deleteShiftInDatabaseById(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.DeleteFromShiftTableById)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt deleteShiftByIdQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.ShiftId)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, deleteShiftInDatabaseById, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func deleteShiftInDatabaseByAssetIdAndTimestamp(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmt := txn.Stmt(statement.DeleteFromShiftTableByAssetIDAndBeginTimestamp)
	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt deleteShiftByAssetIdAndBeginTimestampQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		_, err = stmt.Exec(pt.DBAssetID, pt.BeginTimeStampMs)
		if err != nil {
			faultyItems = append(faultyItems, item)
			err = nil
			continue

		} else {
			workingItems = append(workingItems, item)
		}
	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, deleteShiftInDatabaseByAssetIdAndTimestamp, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err
}

func modifyInDatabaseModifyCountAndScrap(items []*goque.PriorityItem, recursionDepth int) (faultyItems []*goque.PriorityItem, err error) {
	if len(items) == 0 {
		faultyItems = []*goque.PriorityItem{}
		return
	}

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		faultyItems = items

		return
	}

	//These statements' auto close
	stmtCS := txn.Stmt(statement.UpdateCountTableSetCountAndScrapByAssetId)
	stmtC := txn.Stmt(statement.UpdateCountTableSetCountByAssetId)
	stmtS := txn.Stmt(statement.UpdateCountTableSetScrapByAssetId)

	var workingItems []*goque.PriorityItem

	for _, item := range items {
		var pt modifyProducesPieceQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// pt.Count is -1, if not modified by user
		if pt.Count > 0 {
			// pt.Scrap is -1, if not modified by user
			if pt.Scrap > 0 {
				zap.S().Debugf("CS !", pt.Count, pt.Scrap, pt.DBAssetID)
				_, err = stmtCS.Exec(pt.Count, pt.Scrap, pt.DBAssetID)

				if err != nil {
					faultyItems = append(faultyItems, item)

					err = nil
					continue
				} else {
					workingItems = append(workingItems, item)
				}
			} else {
				zap.S().Debugf("C !", pt.Count, pt.DBAssetID)
				_, err = stmtC.Exec(pt.Count, pt.DBAssetID)

				if err != nil {
					faultyItems = append(faultyItems, item)

					err = nil
					continue
				} else {
					workingItems = append(workingItems, item)
				}
			}
		} else {
			// pt.Scrap is -1, if not modified by user
			if pt.Scrap > 0 {
				zap.S().Debugf("S !", pt.Scrap, pt.DBAssetID)
				_, err = stmtS.Exec(pt.Scrap, pt.DBAssetID)

				if err != nil {
					faultyItems = append(faultyItems, item)

					err = nil
					continue
				} else {
					workingItems = append(workingItems, item)
				}
			} else {
				zap.S().Errorf("Invalid amount for Count and Script")
			}
		}

	}

	faultyItems, innerFaultyItems, err2 := CommitWorking(items, faultyItems, txn, workingItems, modifyInDatabaseModifyCountAndScrap, recursionDepth)
	if err2 != nil {
		return innerFaultyItems, err2
	}

	return faultyItems, err

}
