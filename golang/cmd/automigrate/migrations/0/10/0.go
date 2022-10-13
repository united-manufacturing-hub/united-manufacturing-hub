package _10

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/automigrate/database"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"sync"
)

func GetTx(db *sql.DB) *sql.Tx {
	tx, err := db.Begin()
	if err != nil {
		zap.S().Fatalf("Error while opening transaction: %v", err)
	}
	return tx
}

func V0x10x0(db *sql.DB) error {
	zap.S().Infof("Applying migration 0.10.0")

	zap.S().Infof("Creating new tables")
	txCNT := GetTx(db)
	err := createNewTables(txCNT)
	if err != nil {
		errX := txCNT.Rollback()
		if errX != nil {
			zap.S().Errorf("Error while rolling back transaction: %v", err)
		}
		zap.S().Fatalf("Error while creating new tables: %v", err)
	}
	errX := txCNT.Commit()
	if errX != nil {
		zap.S().Errorf("Error while committing back transaction: %v", err)
	}
	zap.S().Infof("Created new tables")

	// Skip if asset table is missing
	var exists bool
	exists, err = database.TableExists(db, "assettable")
	if err != nil {
		zap.S().Fatalf("Error while checking if asset table exists: %v", err)
	}
	if !exists {
		zap.S().Warnf("Asset table does not exist, skipping migration")
		return nil
	}

	zap.S().Infof("Migrating asset table")
	txAT := GetTx(db)
	err = migrateAssetTable(txAT)
	if err != nil {
		errX = txAT.Rollback()
		if errX != nil {
			zap.S().Errorf("Error while rolling back transaction: %v", err)
		}
		zap.S().Fatalf("Error while migrating asset table: %v", err)
	}
	errX = txAT.Commit()
	if errX != nil {
		zap.S().Errorf("Error while rolling back transaction: %v", err)
	}
	zap.S().Infof("Migrated asset table")

	zap.S().Infof("Migrating value names")
	err = createProcessValueNameEntriesFromPVandPVS(db)
	if err != nil {
		zap.S().Fatalf("Error while migrating value names: %v", err)
	}
	zap.S().Infof("Migrated value names")

	zap.S().Infof("Migrating other tables to new ids")
	txOT := GetTx(db)
	err = migrateOtherTables(txOT)
	if err != nil {
		errX = txOT.Rollback()
		if errX != nil {
			zap.S().Errorf("Error while rolling back transaction: %v", err)
		}
		zap.S().Fatalf("Error while migrating other tables: %v", err)
	}
	errX = txOT.Commit()
	if errX != nil {
		zap.S().Errorf("Error while rolling back transaction: %v", err)
	}
	zap.S().Infof("Migrated other tables to new ids")

	// Drop asset table
	zap.S().Infof("Dropping old tables")
	txDOT := GetTx(db)
	err = dropOldTables(txDOT)
	if err != nil {
		errX = txDOT.Rollback()
		if errX != nil {
			zap.S().Errorf("Error while rolling back transaction: %v", err)
		}
		zap.S().Fatalf("Error while dropping old tables: %v", err)
	}
	errX = txDOT.Commit()
	if errX != nil {
		zap.S().Errorf("Error while rolling back transaction: %v", err)
	}

	zap.S().Infof("Applied migration 0.10.0")

	return nil
}

func dropOldTables(db *sql.Tx) error {
	zap.S().Infof("Dropping asset table")
	_, err := db.Exec("DROP TABLE IF EXISTS assettable RESTRICT ")
	if err != nil {
		zap.S().Errorf("Error while dropping asset table: %v", err)
	}
	return err
}

type AssetTable struct {
	assetid  string
	location string
	customer string
	shaSum   string
	id       int
}

var pvNMap map[string]int
var pvNMapRWMutex = sync.RWMutex{}

var assetIdMap = make(map[int]int)
var assetIdMapRWMutex = sync.RWMutex{}

func migrateAssetTable(db *sql.Tx) error {
	//assetTable.customer -> enterpriseTable.name
	//assetTable.location -> siteTable.name
	//assetTable.assetid -> workCellTable.name
	//areaTable.name -> defaultArea + SHA256(assetTable.customer, assetTable.location)
	//productionLineTable.name -> defaultProductionLine + SHA256(assetTable.customer, assetTable.location)

	// Get all assets
	rows, err := db.Query("SELECT * FROM assetTable")
	if err != nil {
		return err
	}
	defer rows.Close()

	var assets []AssetTable

	for rows.Next() {
		var row AssetTable
		err = rows.Scan(&row.id, &row.assetid, &row.location, &row.customer)
		if err != nil {
			return err
		}
		// Calculate SHA256

		var hasher = sha256.New()
		hasher.Write([]byte(row.customer))
		hasher.Write([]byte(row.location))
		// Convert to hex
		row.shaSum = fmt.Sprintf("%x", hasher.Sum(nil))
		assets = append(assets, row)
	}

	// Map of old asset id to new asset id

	// Insert all assets into new tables
	for _, asset := range assets {
		// Insert enterprise
		_, err = db.Exec("INSERT INTO enterpriseTable (name) VALUES ($1) ON CONFLICT do nothing ", asset.customer)
		if err != nil {
			zap.S().Errorf("Error while inserting enterprise: %v", err)
			return err
		}

		// Insert site
		_, err = db.Exec(
			"INSERT INTO siteTable (name, enterpriseId) VALUES ($1, (SELECT id FROM enterpriseTable WHERE name = $2)) ON CONFLICT do nothing ",
			asset.location,
			asset.customer)
		if err != nil {
			zap.S().Errorf("Error while inserting site: %v", err)
			return err
		}

		// Insert area
		_, err = db.Exec(
			"INSERT INTO areaTable (name, siteId) VALUES ($1, (SELECT id FROM siteTable WHERE name = $2 AND enterpriseid = (SELECT id FROM enterprisetable WHERE name = $3))) ON CONFLICT do nothing ",
			"defaultArea"+asset.shaSum,
			asset.location,
			asset.customer)
		if err != nil {
			zap.S().Errorf("Error while inserting area: %v", err)
			return err
		}

		// Insert production line
		_, err = db.Exec(
			"INSERT INTO productionLineTable (name, areaId) VALUES ($1, (SELECT id FROM areaTable WHERE name = $2)) ON CONFLICT do nothing ",
			"defaultProductionLine"+asset.shaSum,
			"defaultArea"+asset.shaSum)
		if err != nil {
			zap.S().Errorf("Error while inserting production line: %v", err)
			return err
		}

		// Insert work cell
		_, err = db.Exec(
			"INSERT INTO workCellTable (name, productionLineId) VALUES ($1, (SELECT id FROM productionLineTable WHERE name = $2)) ON CONFLICT do nothing ",
			asset.assetid,
			"defaultProductionLine"+asset.shaSum)
		if err != nil {
			zap.S().Errorf("Error while inserting work cell: %v", err)
			return err
		}

		// Get new asset id
		var newAssetId int
		err = db.QueryRow(
			"SELECT id FROM workCellTable WHERE name = $1 AND productionLineId = (SELECT id FROM productionLineTable WHERE name = $2) ",
			asset.assetid,
			"defaultProductionLine"+asset.shaSum).Scan(&newAssetId)

		if err != nil {
			zap.S().Errorf("Error while getting new asset id: %v", err)
			return err
		}
		// Add to map
		assetIdMapRWMutex.Lock()
		assetIdMap[asset.id] = newAssetId
		assetIdMapRWMutex.Unlock()
	}

	return nil
}

func createProcessValueNameEntriesFromPVandPVS(db *sql.DB) error {
	// check if valuename exists in processvalue table

	txPV := GetTx(db)
	pvNeedsMigration, err := database.CheckIfColumnExists("valuename", "processvaluetable", txPV)
	if err != nil {
		zap.S().Errorf("Error while checking if column exists: %v", err)
		return err
	}
	err = txPV.Commit()
	if err != nil {
		zap.S().Errorf("Error while committing transaction: %v", err)
		return err
	}

	txPVS := GetTx(db)
	pvsNeedsMigration, err := database.CheckIfColumnExists("valuename", "processvaluestringtable", txPVS)
	if err != nil {
		zap.S().Errorf("Error while checking if column exists: %v", err)
		return err
	}
	err = txPVS.Commit()
	if err != nil {
		zap.S().Errorf("Error while committing transaction: %v", err)
		return err
	}

	if !pvNeedsMigration && !pvsNeedsMigration {
		zap.S().Info("No migration needed for processvalue and processvaluestring table")
		return nil
	}

	processValueNames := make([]string, 0)

	var rows *sql.Rows
	if pvNeedsMigration {
		zap.S().Infof("Getting process values names")
		// Get all distinct process value names
		rows, err = db.Query("SELECT DISTINCT valuename FROM processValueTable")
		if err != nil {
			zap.S().Errorf("Error while getting process value names: %v", err)
			return err
		}
		zap.S().Infof("Scanning process values names")
		for rows.Next() {
			var processValueName string
			err = rows.Scan(&processValueName)
			if err != nil {
				zap.S().Errorf("Error while scanning process value name: %v", err)
				return err
			}
			processValueNames = append(processValueNames, processValueName)
		}
	}

	if pvsNeedsMigration {
		zap.S().Infof("Getting process value string names")
		// Get all distinct process value string names
		rows, err = db.Query("SELECT DISTINCT valuename FROM processValueStringTable")
		if err != nil {
			return err
		}
		zap.S().Infof("Scanning process value string names")
		for rows.Next() {
			var processValueName string
			err = rows.Scan(&processValueName)
			if err != nil {
				zap.S().Errorf("Error while scanning process value string name: %v", err)
				return err
			}
			processValueNames = append(processValueNames, processValueName)
		}
	}

	// Check if length of processValueNameTable is equal to processValueNames
	var processValueNameTableLength int
	err = db.QueryRow("SELECT COUNT(1) FROM processValueNameTable").Scan(&processValueNameTableLength)
	if err != nil {
		return err
	}

	var threads = 90
	var count int
	if processValueNameTableLength > len(processValueNames) {
		count = processValueNameTableLength
	} else {
		count = len(processValueNames)
	}
	zap.S().Infof("Count of process value names: %d", count)

	if processValueNameTableLength != len(processValueNames) {
		var tx *sql.Tx
		tx, err = db.Begin()
		if err != nil {
			return err
		}
		zap.S().Infof(
			"Creating process value name entries (%d vs %d)",
			processValueNameTableLength,
			len(processValueNames))
		// Create tmp_processValueNameTable
		_, err = tx.Exec("CREATE TEMP TABLE tmp_processvaluenametable (LIKE processValueNameTable INCLUDING DEFAULTS) ON COMMIT DROP")
		if err != nil {
			return err
		}

		zap.S().Infof("Inserting %d process value names", len(processValueNames))
		// insert process value names set and get there ids

		var prepCopy *sql.Stmt
		prepCopy, err = tx.Prepare(pq.CopyIn("tmp_processvaluenametable", "name"))
		if err != nil {
			zap.S().Errorf("Error while preparing copy: %v", err)
			return err
		}

		// deduplicate processValueNames
		zap.S().Infof("Deduplicating process value names")
		processValueNames = deduplicate(processValueNames)

		zap.S().Infof("Inserting process value names into work channel")
		var cvChan = make(chan string, len(processValueNames))
		for i := 0; i < len(processValueNames); i++ {
			cvChan <- processValueNames[i]
		}

		var wg sync.WaitGroup
		wg.Add(threads)
		zap.S().Infof("Starting %d threads", threads)
		for i := 0; i < threads; i++ {
			go concurrentInsertScan(cvChan, prepCopy, &wg)
		}
		wg.Wait()
		zap.S().Infof("All threads finished")
		err = prepCopy.Close()
		if err != nil {
			zap.S().Errorf("Error while closing copy: %v", err)
			return err
		}
		// Copy tmp table to processValueNameTable
		zap.S().Infof("Copying tmp table to processValueNameTable")
		_, err = tx.Exec("INSERT INTO processValueNameTable (name) (SELECT name FROM tmp_processvaluenametable) ON CONFLICT do nothing")
		if err != nil {
			zap.S().Errorf("Error while copying tmp table to processValueNameTable: %v", err)
			return err
		}

		err = tx.Commit()
		if err != nil {
			zap.S().Errorf("Error while committing: %v", err)
			return err
		}

	} else {
		zap.S().Infof("Process value names already in database")
	}

	zap.S().Infof("Pre-allocating map for %d process value names", count)
	pvNMapRWMutex.Lock()
	pvNMap = make(map[string]int, count)
	pvNMapRWMutex.Unlock()

	// Divide count by threads
	var chunkSize = count / threads
	var remainder = count % threads

	zap.S().Infof("Preparing name select statement")
	// prepare select statement with chunk size
	var prepSelect *sql.Stmt
	prepSelect, err = db.Prepare("SELECT id, name FROM processValueNameTable LIMIT $1 OFFSET $2")
	if err != nil {
		zap.S().Errorf("Error while preparing select: %v", err)
		return err
	}
	zap.S().Infof("Finished preparing name select statement")

	var wg sync.WaitGroup
	zap.S().Infof("Starting %d threads", threads)
	wg.Add(threads)
	for i := 0; i < threads; i++ {
		if i == threads-1 {
			go concurrentGetProcessValueNames(i*chunkSize, (i+1)*chunkSize+remainder, &wg, prepSelect)
		} else {
			go concurrentGetProcessValueNames(i*chunkSize, (i+1)*chunkSize, &wg, prepSelect)
		}
	}
	wg.Wait()

	return nil
}

func concurrentGetProcessValueNames(offset int, limit int, s *sync.WaitGroup, prepSelect *sql.Stmt) {
	defer s.Done()
	rows, err := prepSelect.Query(limit, offset)
	if err != nil {
		zap.S().Errorf("Error while querying: %v", err)
		return
	}
	localMap := make(map[string]int, limit)
	for rows.Next() {
		var id int
		var name string
		err = rows.Scan(&id, &name)
		if err != nil {
			zap.S().Errorf("Error while scanning: %v", err)
			return
		}
		localMap[name] = id
	}
	pvNMapRWMutex.Lock()
	for k, v := range localMap {
		pvNMap[k] = v
	}
	pvNMapRWMutex.Unlock()
	zap.S().Infof("Finished thread %d -> %d", offset, limit)
}

func deduplicate[T comparable](s []T) []T {
	inResult := make(map[T]bool)
	var result []T
	for _, str := range s {
		if _, ok := inResult[str]; !ok {
			inResult[str] = true
			result = append(result, str)
		}
	}
	return result
}

var concurrentInsertScanCounterAtomic = atomic.NewInt32(0)

func concurrentInsertScan(cvChan chan string, prepCopy *sql.Stmt, s *sync.WaitGroup) {

	var err error

	for {

		select {
		case x, ok := <-cvChan:
			if ok {
				_, err = prepCopy.Exec(x)
				if err != nil {
					var sqlErr *pq.Error
					okC := errors.As(err, &sqlErr)
					if !okC {
						zap.S().Fatalf("Error while casting error to sql error: %v", err)
					}
					// if error is unique_violation, get id of process value name
					if sqlErr.Code == "23505" {
						continue
					} else {
						zap.S().Fatalf("Error while inserting process value name: %v", err)
					}
				}
				concurrentInsertScanCounterAtomic.Add(1)
				if current := concurrentInsertScanCounterAtomic.Load(); current%10000 == 0 {
					zap.S().Infof("Inserted %d process value names", current)
				}
			} else {
				s.Done()
				return
			}
		default:
			s.Done()
			return
		}
	}

}

func createNewTables(db *sql.Tx) error {
	var creationCommand = `
    CREATE TABLE IF NOT EXISTS enterpriseTable (
        id SERIAL PRIMARY KEY,
        name text UNIQUE NOT NULL
    );

    CREATE TABLE IF NOT EXISTS siteTable (
        id SERIAL PRIMARY KEY,
        name text NOT NULL,
        enterpriseId int NOT NULL REFERENCES enterpriseTable (id),
        unique (name, enterpriseId)
    );

    CREATE TABLE IF NOT EXISTS areaTable (
        id SERIAL PRIMARY KEY,
        name text NOT NULL,
        siteId int NOT NULL REFERENCES siteTable (id),
        unique (name, siteId)
    );

    CREATE TABLE IF NOT EXISTS productionLineTable (
        id SERIAL PRIMARY KEY,
        name text NOT NULL,
        areaId int NOT  NULL REFERENCES areaTable (id),
        unique (name, areaId)
    );

    CREATE TABLE IF NOT EXISTS workCellTable (
        id SERIAL PRIMARY KEY,
        name text NOT NULL,
        productionLineId int NOT  NULL REFERENCES workCellTable (id),
        unique (name, productionLineId)
    );

    CREATE TABLE IF NOT EXISTS processValueNameTable (
        id SERIAL PRIMARY KEY,
        name text UNIQUE NOT NULL
    );

    CREATE INDEX IF NOT EXISTS processvaluenametable_name_idx ON processValueNameTable (name);
`

	_, err := db.Exec(creationCommand)
	return err
}

func migrateOtherTables(db *sql.Tx) error {
	err := migrateComponentTable(db)
	if err != nil {
		zap.S().Errorf("Error while migrating component table: %v", err)
		return err
	}
	err = migrateCountTable(db)
	if err != nil {
		zap.S().Errorf("Error while migrating count table: %v", err)
		return err
	}
	err = migrateOrderTable(db)
	if err != nil {
		zap.S().Errorf("Error while migrating order table: %v", err)
		return err
	}
	err = migrateProcessValueTable(db)
	if err != nil {
		zap.S().Errorf("Error while migrating process value table: %v", err)
		return err
	}
	err = migrateProcessValueStringTable(db)
	if err != nil {
		zap.S().Errorf("Error while migrating process value string table: %v", err)
		return err
	}
	err = migrateProductTable(db)
	if err != nil {
		zap.S().Errorf("Error while migrating product table: %v", err)
		return err
	}
	err = migrateShiftTable(db)
	if err != nil {
		zap.S().Errorf("Error while migrating shift table: %v", err)
		return err
	}
	err = migrateStateTable(db)
	if err != nil {
		zap.S().Errorf("Error while migrating state table: %v", err)
		return err
	}
	err = migrateUniqueProductTable(db)
	if err != nil {
		zap.S().Errorf("Error while migrating unique product table: %v", err)
		return err
	}
	return nil
}

func migrateUniqueProductTable(db *sql.Tx) error {
	// Check if asset_id column exists
	assetIdExists, err := database.CheckIfColumnExists("asset_id", "uniqueproducttable", db)
	if err != nil {
		zap.S().Errorf("Error while checking if asset_id column exists: %v", err)
		return err
	}
	if !assetIdExists {
		zap.S().Info("Asset_id column does not exist in uniqueproducttable. Skipping migration")
		return nil
	}

	err = database.DropTableConstraint("uniqueproducttable_asset_id_fkey", "uniqueproducttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id foreign key constraint: %v", err)
		return err
	}

	err = database.AddIntColumn("workCellId", "uniqueproducttable", db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId column: %v", err)
		return err
	}
	err = database.AddFKConstraint(
		"uniqueproducttable_workcellid_fkey",
		"uniqueproducttable",
		"workCellId",
		"workCellTable",
		"id",
		db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId foreign key constraint: %v", err)
		return err
	}

	err = AssetIdToWorkCellIdConverter("uniqueproducttable", db)
	if err != nil {
		zap.S().Errorf("Error while converting asset_id to workCellId: %v", err)
		return err
	}

	err = database.DropColumn("asset_id", "uniqueproducttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id column: %v", err)
		return err
	}
	return nil
}

func migrateStateTable(db *sql.Tx) error {

	// Check if asset_id column exists
	assetIdExists, err := database.CheckIfColumnExists("asset_id", "statetable", db)
	if err != nil {
		zap.S().Errorf("Error while checking if asset_id column exists: %v", err)
		return err
	}
	if !assetIdExists {
		zap.S().Info("Asset_id column does not exist in statetable. Skipping migration")
		return nil
	}

	err = database.DropTableConstraint("statetable_asset_id_fkey", "statetable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id foreign key constraint: %v", err)
		return err
	}

	err = database.AddIntColumn("workCellId", "statetable", db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId column: %v", err)
		return err
	}

	err = database.AddFKConstraint(
		"statetable_workcellid_fkey",
		"statetable",
		"workCellId",
		"workCellTable",
		"id",
		db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId foreign key constraint: %v", err)
		return err
	}
	err = AssetIdToWorkCellIdConverter("statetable", db)
	if err != nil {
		zap.S().Errorf("Error while converting asset_id to workCellId: %v", err)
		return err
	}
	err = database.DropColumn("asset_id", "statetable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id column: %v", err)
		return err
	}
	return nil
}

func migrateShiftTable(db *sql.Tx) error {
	// Check if asset_id column exists
	assetIdExists, err := database.CheckIfColumnExists("asset_id", "shifttable", db)
	if err != nil {
		zap.S().Errorf("Error while checking if asset_id column exists: %v", err)
		return err
	}
	if !assetIdExists {
		zap.S().Info("Asset_id column does not exist in shifttable. Skipping migration")
		return nil
	}

	err = database.DropTableConstraint("shifttable_asset_id_fkey", "shifttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id foreign key constraint: %v", err)
		return err
	}

	err = database.AddIntColumn("workCellId", "shifttable", db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId column: %v", err)
		return err
	}

	err = database.AddFKConstraint(
		"shifttable_workcellid_fkey",
		"shifttable",
		"workCellId",
		"workCellTable",
		"id",
		db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId foreign key constraint: %v", err)
		return err
	}

	err = AssetIdToWorkCellIdConverter("shifttable", db)
	if err != nil {
		zap.S().Errorf("Error while converting asset_id to workCellId: %v", err)
		return err
	}

	err = database.DropColumn("asset_id", "shifttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id column: %v", err)
		return err
	}
	return nil
}

func migrateProductTable(db *sql.Tx) error {
	// Check if asset_id column exists
	assetIdExists, err := database.CheckIfColumnExists("asset_id", "producttable", db)
	if err != nil {
		zap.S().Errorf("Error while checking if asset_id column exists: %v", err)
		return err
	}
	if !assetIdExists {
		zap.S().Info("Asset_id column does not exist in producttable. Skipping migration")
		return nil
	}

	err = database.DropTableConstraint("producttable_asset_id_fkey", "producttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id foreign key constraint: %v", err)
		return err
	}

	err = database.AddIntColumn("workCellId", "producttable", db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId column: %v", err)
		return err
	}

	err = database.AddUniqueConstraint(
		"producttable_product_name_workCellId_key",
		"producttable",
		[]string{"product_name", "workCellId"},
		db)
	if err != nil {
		zap.S().Errorf("Error while adding unique constraint on producttable: %v", err)
		return err
	}

	err = database.AddFKConstraint(
		"producttable_workcellid_fkey",
		"producttable",
		"workCellId",
		"workCellTable",
		"id",
		db)

	if err != nil {

		zap.S().Errorf("Error while adding workCellId foreign key constraint: %v", err)
		return err
	}

	err = AssetIdToWorkCellIdConverter("producttable", db)
	if err != nil {
		zap.S().Errorf("Error while converting asset_id to workCellId: %v", err)
		return err
	}

	err = database.DropColumn("asset_id", "producttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id column: %v", err)
		return err
	}
	return nil
}

func AssetIdToWorkCellIdConverter(tableName string, db *sql.Tx) error {
	var err error
	assetIdMapRWMutex.RLock()
	defer assetIdMapRWMutex.RUnlock()
	for oldId, newId := range assetIdMap {
		_, err = db.Exec(fmt.Sprintf("UPDATE %s SET workCellId = %d WHERE asset_id = %d", tableName, newId, oldId))
		if err != nil {
			return err
		}
	}
	return nil
}

func migrateComponentTable(db *sql.Tx) error {
	// Check if asset_id column exists
	assetIdExists, err := database.CheckIfColumnExists("asset_id", "componenttable", db)
	if err != nil {
		zap.S().Errorf("Error while checking if asset_id column exists: %v", err)
		return err
	}
	if !assetIdExists {
		zap.S().Info("Asset_id column does not exist in componenttable. Skipping migration")
		return nil
	}

	err = database.DropTableConstraint("componenttable_asset_id_fkey", "componenttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id foreign key constraint: %v", err)
		return err
	}

	err = database.AddIntColumn("workCellId", "componenttable", db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId column: %v", err)
		return err
	}
	err = database.AddUniqueConstraint(
		"componenttable_workCellId_componentname_key",
		"componenttable",
		[]string{"workCellId", "componentname"},
		db)
	if err != nil {
		zap.S().Errorf("Error while adding unique constraint: %v", err)
		return err
	}
	err = database.AddFKConstraint(
		"componenttable_workcellid_fkey",
		"componenttable",
		"workCellId",
		"workCellTable",
		"id",
		db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId foreign key constraint: %v", err)
		return err
	}

	err = AssetIdToWorkCellIdConverter("componenttable", db)
	if err != nil {
		zap.S().Errorf("Error while converting asset_id to workCellId: %v", err)
		return err
	}

	err = database.DropTableConstraint("componenttable_asset_id_componentname_key", "componenttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id unique constraint: %v", err)
		return err
	}
	err = database.DropColumn("asset_id", "componenttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id column: %v", err)
		return err
	}

	return nil
}

func migrateCountTable(db *sql.Tx) error {
	// Check if asset_id column exists
	assetIdExists, err := database.CheckIfColumnExists("asset_id", "counttable", db)
	if err != nil {
		zap.S().Errorf("Error while checking if asset_id column exists: %v", err)
		return err
	}
	if !assetIdExists {
		zap.S().Info("Asset_id column does not exist in counttable. Skipping migration")
		return nil
	}

	err = database.DropTableConstraint("counttable_asset_id_fkey", "counttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id foreign key constraint: %v", err)
		return err
	}

	err = database.DropTableConstraint("counttable_timestamp_asset_id_key", "counttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping timestamp_asset_id unique constraint: %v", err)
		return err
	}

	err = database.AddIntColumn("workCellId", "counttable", db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId column: %v", err)
		return err
	}
	err = database.AddUniqueConstraint(
		"counttable_workcellid_timestamp_key",
		"counttable",
		[]string{"workCellId", "timestamp"},
		db)
	if err != nil {
		zap.S().Errorf("Error while adding unique constraint: %v", err)
		return err
	}
	err = database.AddFKConstraint("counttable_workcellid_fkey", "counttable", "workCellId", "workCellTable", "id", db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId foreign key constraint: %v", err)
		return err
	}

	err = AssetIdToWorkCellIdConverter("counttable", db)
	if err != nil {
		zap.S().Errorf("Error while converting asset_id to workCellId: %v", err)
		return err
	}

	err = database.DropColumn("asset_id", "counttable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id column: %v", err)
		return err
	}
	return nil
}

func migrateOrderTable(db *sql.Tx) error {
	// Check if asset_id column exists
	assetIdExists, err := database.CheckIfColumnExists("asset_id", "ordertable", db)
	if err != nil {
		zap.S().Errorf("Error while checking if asset_id column exists: %v", err)
		return err
	}
	if !assetIdExists {
		zap.S().Info("Asset_id column does not exist in ordertable. Skipping migration")
		return nil
	}

	err = database.DropTableConstraint("ordertable_asset_id_fkey", "ordertable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id foreign key constraint: %v", err)
		return err
	}

	err = database.AddIntColumn("workCellId", "ordertable", db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId column: %v", err)
		return err
	}
	err = database.AddUniqueConstraint(
		"ordertable_workCellId_ordername_key",
		"ordertable",
		[]string{"workCellId", "order_name"},
		db)
	if err != nil {
		zap.S().Errorf("Error while adding unique constraint: %v", err)
		return err
	}
	err = database.AddFKConstraint("ordertable_workcellid_fkey", "ordertable", "workCellId", "workCellTable", "id", db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId foreign key constraint: %v", err)
		return err
	}

	err = AssetIdToWorkCellIdConverter("ordertable", db)
	if err != nil {
		zap.S().Errorf("Error while converting asset_id to workCellId: %v", err)
		return err
	}

	err = database.DropColumn("asset_id", "ordertable", db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id column: %v", err)
		return err
	}
	return nil
}

func migrateProcessValueTable(db *sql.Tx) error {
	return migrateProcessXTable("processvaluetable", db)
}

func migrateProcessValueStringTable(db *sql.Tx) error {
	return migrateProcessXTable("processvaluestringtable", db)
}

func migrateProcessXTable(tableName string, db *sql.Tx) error {
	// Check if asset_id column exists
	assetIdExists, err := database.CheckIfColumnExists("asset_id", tableName, db)
	if err != nil {
		zap.S().Errorf("Error while checking if asset_id column exists: %v", err)
		return err
	}
	if !assetIdExists {
		zap.S().Info("Asset_id column does not exist in " + tableName + ". Skipping migration")
		return nil
	}

	// Drop asset_id foreign key constraint
	err = database.DropTableConstraint(tableName+"_asset_id_fkey", tableName, db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id foreign key constraint: %v", err)
		return err
	}

	// Drop timestamp,asset_id,valuename unique constraint
	err = database.DropTableConstraint(tableName+"_timestamp_asset_id_valuename_key", tableName, db)
	if err != nil {
		zap.S().Errorf("Error while dropping timestamp_asset_id unique constraint: %v", err)
		return err
	}

	// Add workCellId column
	err = database.AddIntColumn("workCellId", tableName, db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId column: %v", err)
		return err
	}

	// Add valueNameId column
	err = database.AddIntColumn("valueNameId", tableName, db)
	if err != nil {
		zap.S().Errorf("Error while adding valueNameId column: %v", err)
		return err
	}

	// Add unique constraint
	err = database.AddUniqueConstraint(
		tableName+"_workcellid_timestamp_valuenameid_key",
		tableName,
		[]string{"workCellId", "timestamp", "valueNameId"},
		db)
	if err != nil {
		zap.S().Errorf("Error while adding unique constraint: %v", err)
		return err
	}

	// Add foreign key constraint for workCellId
	err = database.AddFKConstraint(
		tableName+"_workcellid_fkey",
		tableName,
		"workCellId",
		"workCellTable",
		"id",
		db)
	if err != nil {
		zap.S().Errorf("Error while adding workCellId foreign key constraint: %v", err)
		return err
	}

	// Add foreign key constraint for valueNameId
	err = database.AddFKConstraint(
		tableName+"_valuenameid_fkey",
		tableName,
		"valueNameId",
		"processvaluenametable",
		"id",
		db)
	if err != nil {
		zap.S().Errorf("Error while adding valueNameId foreign key constraint: %v", err)
		return err
	}

	// Convert asset_id to workCellId and valuename to valueNameId
	// This might not work with the new FK constraint from valueNameId
	err = AssetIdToWorkCellIdConverter(tableName, db)
	if err != nil {
		zap.S().Errorf("Error while converting asset_id to workCellId: %v", err)
		return err
	}

	err = ValueNameToValueNameIdConverter(tableName, db)
	if err != nil {
		zap.S().Errorf("Error while converting valuename to valueNameId: %v", err)
		return err
	}

	// Drop valuename column
	err = database.DropColumn("valuename", tableName, db)
	if err != nil {
		zap.S().Errorf("Error while dropping valuename column: %v", err)
		return err
	}

	// Drop asset_id column
	err = database.DropColumn("asset_id", tableName, db)
	if err != nil {
		zap.S().Errorf("Error while dropping asset_id column: %v", err)
		return err
	}

	return nil
}

func ValueNameToValueNameIdConverter(originTable string, db *sql.Tx) error {
	var err error
	pvNMapRWMutex.RLock()
	defer pvNMapRWMutex.RUnlock()
	for name, id := range pvNMap {
		_, err = db.Exec(fmt.Sprintf("UPDATE %s SET valuenameid = %d WHERE valuename = '%s'", originTable, id, name))
		if err != nil {
			zap.S().Errorf("Error while converting valuename to valueNameId: %v", err)
			return err
		}
	}
	return nil
}
