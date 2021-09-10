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

// CommitOrRollbackOnError runs at the end of every database function
// Either commits or rolls back the transaction, depending on if the tx was successful
func CommitOrRollbackOnError(txn *sql.Tx, errIn error) (errOut error) {
	if txn == nil {
		PQErrorHandling("Transaction is nil", errIn)
		return
	}

	if errIn != nil {
		zap.S().Debugf("Got error from callee: %s", errIn)
		errOut = txn.Rollback()
		return
	}

	if isDryRun {
		zap.S().Debugf("PREPARED STATEMENT")
		errOut = txn.Rollback()
		if errOut != nil {
			if errOut != sql.ErrTxDone {
				PQErrorHandling("txn.Rollback()", errOut)
			} else {
				zap.S().Warnf("%s", errOut)
			}

		}
	} else {
		errOut = txn.Commit()
		if errOut != nil {
			if errOut != sql.ErrTxDone {
				PQErrorHandling("txn.Commit()", errOut)
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
func GetAssetID(customerID string, location string, assetID string) (DBassetID uint32) {
	zap.S().Debugf("[GetUniqueProductID] customerID: %s, location: %s, assetID: %s", customerID, location, assetID)

	// Get from cache if possible
	var cacheHit bool
	DBassetID, cacheHit = internal.GetAssetIDFromCache(customerID, location, assetID)
	if cacheHit { // data found
		// zap.S().Debugf("GetAssetID cache hit")
		return
	}

	err := statement.SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId.QueryRow(assetID, location, customerID).Scan(&DBassetID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found for assetID: %s, location: %s, customerID: %s", assetID, location, customerID)
	} else if err != nil {
		PQErrorHandling("GetAssetID db.QueryRow()", err)
	}

	// Store to cache if not yet existing
	go internal.StoreAssetIDToCache(customerID, location, assetID, DBassetID)
	zap.S().Debugf("Stored AssetID to cache")

	return
}

// GetProductID gets the productID for a asset and a productName from the database
func GetProductID(DBassetID uint32, productName string) (productID int32, err error, success bool) {
	zap.S().Debugf("[GetUniqueProductID] DBassetID: %d, productName: %s", DBassetID, productName)
	success = false

	err = statement.SelectProductIdFromProductTableByAssetIdAndProductName.QueryRow(DBassetID, productName).Scan(&productID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found DBAssetID: %d, productName: %s", DBassetID, productName)
		return
	} else if err != nil {
		PQErrorHandling("GetProductID db.QueryRow()", err)
	}
	success = true

	return
}

// GetComponentID gets the componentID from the database
func GetComponentID(assetID uint32, componentName string) (componentID int32, success bool) {
	zap.S().Debugf("[GetUniqueProductID] assetID: %d, componentName: %s", assetID, componentName)
	success = false
	err := statement.SelectIdFromComponentTableByAssetIdAndComponentName.QueryRow(assetID, componentName).Scan(&componentID)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found assetID: %d, componentName: %s", assetID, componentName)
		return
	} else if err != nil {
		PQErrorHandling("GetComponentID() db.QueryRow()", err)
	}
	success = true
	return
}

func GetUniqueProductID(aid string, DBassetID uint32) (uid uint32, err error, success bool) {
	zap.S().Debugf("[GetUniqueProductID] aid: %s, DBassetID: %d", aid, DBassetID)
	success = false
	err = statement.SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndAssetIdOrderedByTimeStampDesc.QueryRow(aid, DBassetID).Scan(&uid)
	if err == sql.ErrNoRows {
		zap.S().Errorf("No Results Found aid: %s, DBassetID: %d", aid, DBassetID)
		return
	} else if err != nil {
		PQErrorHandling("GetUniqueProductID db.QueryRow()", err)
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
		PQErrorHandling("GetLatestParentUniqueProductID db.QueryRow()", err)
	}
	success = true
	return
}

func CheckIfProductExists(productId int32) (exists bool) {
	txn, err := db.Begin()
	if err != nil {
		return false
	}

	var cnt int32

	stmt := txn.Stmt(statement.SelectProductExists)
	err = stmt.QueryRow(productId).Scan(&cnt)
	if err != nil {
		zap.S().Debugf("Failed to scan rows ", err)
		return false
	}

	return cnt == 1
}

// AddAssetIfNotExisting adds an asset to the db if it is not existing yet
func AddAssetIfNotExisting(assetID string, location string, customerID string) {

	// Get from cache if possible
	var cacheHit bool
	_, cacheHit = internal.GetAssetIDFromCache(customerID, location, assetID)
	if cacheHit { // data found
		zap.S().Debugf("Cache hit for %s", assetID)
		return
	}
	zap.S().Debugf("No Cache hit for %s", assetID)

	txn, err := db.Begin()
	if err != nil {
		return
	}

	defer func() {
		errx := CommitOrRollbackOnError(txn, err)
		if errx != nil {
			err = errx
			return
		}
	}()

	stmt := txn.Stmt(statement.InsertIntoAssetTable)

	_, err = stmt.Exec(assetID, location, customerID)
	if err != nil {
		PQErrorHandling("INSERT INTO ASSETTABLE", err)
	}
}

func storeItemsIntoDatabaseRecommendation(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.InsertIntoRecommendationTable)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseProcessValueFloat64(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
				zap.S().Debugf("Got an error before err = nil: %s", err)
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

func storeItemsIntoDatabaseProcessValueString(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
				zap.S().Debugf("Got an error before err = nil: %s", err)
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

func storeItemsIntoDatabaseProcessValue(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
				zap.S().Debugf("Got an error before err = nil: %s", err)
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

func storeItemsIntoDatabaseCount(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
				zap.S().Debugf("Got an error before err = nil: %s", err)
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

func storeItemsIntoDatabaseState(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.InsertIntoStateTable)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseScrapCount(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.UpdateCountTableScrap)

	for _, item := range items {
		var pt scrapCountQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		// Create statement
		zap.S().Debugf("[PRE]\tstoreItemsIntoDatabaseScrapCount")
		_, err = stmt.Exec(pt.TimestampMs, pt.DBAssetID, pt.Scrap)
		zap.S().Debugf("[POST]\tstoreItemsIntoDatabaseScrapCount")
		if err != nil {
			faultyItems = append(faultyItems, item)
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseUniqueProduct(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.InsertIntoUniqueProductTable)

	for _, item := range items {
		var pt uniqueProductQueue
		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		if CheckIfProductExists(pt.ProductID) {
			// Create statement
			_, err = stmt.Exec(pt.DBAssetID, pt.BeginTimestampMs, NewNullInt64(int64(pt.EndTimestampMs)), pt.ProductID, pt.IsScrap, pt.UniqueProductAlternativeID)
			if err != nil {
				faultyItems = append(faultyItems, item)
				zap.S().Debugf("Got an error before err = nil: %s", err)
				err = nil
				continue

			}
		} else {
			zap.S().Debugf("Product %d does not yet exist", pt.ProductID)
			faultyItems = append(faultyItems, item)
		}

	}
	return
}

func storeItemsIntoDatabaseProductTag(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.InsertIntoProductTagTable)

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
		uid, err, success = GetUniqueProductID(pt.AID, pt.DBAssetID)
		if err != nil || !success {
			zap.S().Errorf("Stopped writing productTag in Database, uid not found. AID: %s, DBAssetID %d", pt.AID, pt.DBAssetID)
			faultyItems = append(faultyItems, item)
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}

		// Create statement
		_, err = stmt.Exec(pt.Name, pt.Value, pt.TimestampMs, uid)
		if err != nil {
			faultyItems = append(faultyItems, item)
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseProductTagString(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.InsertIntoProductTagStringTable)

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
		uid, err, success = GetUniqueProductID(pt.AID, pt.DBAssetID)
		if err != nil || !success {
			zap.S().Errorf("Stopped writing productTag in Database, uid not found. AID: %s, DBAssetID %d", pt.AID, pt.DBAssetID)
			faultyItems = append(faultyItems, item)
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}

		// Create statement
		_, err = stmt.Exec(pt.Name, pt.Value, pt.TimestampMs, uid)
		if err != nil {
			faultyItems = append(faultyItems, item)
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseAddParentToChild(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.InsertIntoProductInheritanceTable)

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
		childUid, err, success = GetUniqueProductID(pt.ChildAID, pt.DBAssetID)
		if err != nil || !success {
			zap.S().Errorf("Stopped writing addParentToChild in Database, childUid not found. ChildAID: %s, DBAssetID: %d", pt.ChildAID, pt.DBAssetID)
			faultyItems = append(faultyItems, item)
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
		var parentUid int32
		parentUid, success = GetLatestParentUniqueProductID(pt.ParentAID, pt.DBAssetID)
		if !success {
			zap.S().Errorf("Stopped writing addParentToChild in Database, parentUid not found. ChildAID: %s, DBAssetID: %d", pt.ChildAID, pt.DBAssetID)
			faultyItems = append(faultyItems, item)
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue
		}

		// Create statement
		zap.S().Debugf("[storeItemsIntoDatabaseAddParentToChild] ParentUID: %d, childUID: %d", parentUid, childUid)
		_, err = stmt.Exec(parentUid, childUid, pt.TimestampMs)
		if err != nil {
			faultyItems = append(faultyItems, item)
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseShift(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.InsertIntoShiftTable)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseUniqueProductScrap(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.UpdateUniqueProductTableSetIsScrap)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseAddProduct(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
	if err != nil {
		zap.S().Errorf("Failed to open txn: %s", err)
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
	stmt := txn.Stmt(statement.InsertIntoProductTable)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseAddOrder(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.InsertIntoOrderTable)

	for _, item := range items {
		var pt addOrderQueue

		err = item.ToObjectFromJSON(&pt)
		if err != nil {
			err = nil
			zap.S().Errorf("Failed to unmarshal item", item)
			continue
		}

		if CheckIfProductExists(pt.ProductID) {
			// Create statement
			_, err = stmt.Exec(pt.OrderName, pt.ProductID, pt.TargetUnits, pt.DBAssetID)
			if err != nil {
				faultyItems = append(faultyItems, item)
				zap.S().Debugf("Got an error before err = nil: %s", err)
				err = nil
				continue

			}
		} else {
			zap.S().Debugf("Product %d does not yet exist", pt.ProductID)
			faultyItems = append(faultyItems, item)
		}
	}
	return
}

func storeItemsIntoDatabaseStartOrder(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.UpdateOrderTableSetBeginTimestamp)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseEndOrder(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.UpdateOrderTableSetEndTimestamp)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func storeItemsIntoDatabaseAddMaintenanceActivity(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.InsertIntoMaintenanceActivities)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func modifyStateInDatabase(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
				err = PQErrorHandlingTransaction("rows.Scan()", err, txn)
				if err != nil {
					return
				}
			}
			LastRowTimestampInt = int64(int(LastRowTimestamp))

			err = val.Close()
			if err != nil {
				err = PQErrorHandlingTransaction("val.Close()", err, txn)
				if err != nil {
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

func deleteShiftInDatabaseById(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.DeleteFromShiftTableById)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func deleteShiftInDatabaseByAssetIdAndTimestamp(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmt := txn.Stmt(statement.DeleteFromShiftTableByAssetIDAndBeginTimestamp)

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
			zap.S().Debugf("Got an error before err = nil: %s", err)
			err = nil
			continue

		}
	}
	return
}

func modifyInDatabaseModifyCountAndScrap(items []*goque.PriorityItem) (faultyItems []*goque.PriorityItem, err error) {

	txn, err := db.Begin()
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
	stmtCS := txn.Stmt(statement.UpdateCountTableSetCountAndScrapByAssetId)
	stmtC := txn.Stmt(statement.UpdateCountTableSetCountByAssetId)
	stmtS := txn.Stmt(statement.UpdateCountTableSetScrapByAssetId)

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
					zap.S().Debugf("Got an error before err = nil: %s", err)
					err = nil
					continue
				}
			} else {
				zap.S().Debugf("C !", pt.Count, pt.DBAssetID)
				_, err = stmtC.Exec(pt.Count, pt.DBAssetID)

				if err != nil {
					faultyItems = append(faultyItems, item)
					zap.S().Debugf("Got an error before err = nil: %s", err)
					err = nil
					continue
				}
			}
		} else {
			// pt.Scrap is -1, if not modified by user
			if pt.Scrap > 0 {
				zap.S().Debugf("S !", pt.Scrap, pt.DBAssetID)
				_, err = stmtS.Exec(pt.Scrap, pt.DBAssetID)

				if err != nil {
					faultyItems = append(faultyItems, item)
					zap.S().Debugf("Got an error before err = nil: %s", err)
					err = nil
					continue
				}
			} else {
				zap.S().Errorf("Invalid amount for Count and Script")
			}
		}

	}
	return
}
