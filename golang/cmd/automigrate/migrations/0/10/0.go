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
	"math/rand"
	"sync"
)

func V0x10x0(db *sql.DB) error {
	zap.S().Infof("Applying migration 0.10.0")

	// Open transaction
	tx, err := db.Begin()
	if err != nil {
		zap.S().Fatalf("Error while opening transaction: %v", err)
	}

	zap.S().Infof("Creating new tables")
	err = createNewTables(tx)
	if err != nil {
		tx.Rollback()
		zap.S().Fatalf("Error while creating new tables: %v", err)
	}
	zap.S().Infof("Created new tables")

	zap.S().Infof("Migrating asset table")
	err = migrateAssetTable(tx)
	if err != nil {
		tx.Rollback()
		zap.S().Fatalf("Error while migrating asset table: %v", err)
	}
	zap.S().Infof("Migrated asset table")

	zap.S().Infof("Migrating value names")
	err = createProcessValueNameEntriesFromPVandPVS(tx)
	if err != nil {
		tx.Rollback()
		zap.S().Fatalf("Error while migrating value names: %v", err)
	}
	zap.S().Infof("Migrated value names")

	zap.S().Infof("Migrating other tables to new ids")
	err = migrateOtherTables(tx)
	if err != nil {
		tx.Rollback()
		zap.S().Fatalf("Error while migrating other tables: %v", err)
	}
	zap.S().Infof("Migrated other tables to new ids")

	err = tx.Commit()
	if err != nil {
		zap.S().Fatalf("Error while committing transaction: %v", err)
	}

	return errors.New("not implemented")
}

type AssetTable struct {
	assetid  string
	location string
	customer string
	shaSum   string
	id       int
}

var pvNMap = make(map[string]int)
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

func createProcessValueNameEntriesFromPVandPVS(db *sql.Tx) error {
	processValueNames := make([]string, 0)

	zap.S().Infof("Getting process values names")
	// Get all distinct process value names
	rows, err := db.Query("SELECT DISTINCT valuename FROM processValueTable")
	if err != nil {
		return err
	}
	zap.S().Infof("Scanning process values names")
	for rows.Next() {
		var processValueName string
		err = rows.Scan(&processValueName)
		if err != nil {
			return err
		}
		processValueNames = append(processValueNames, processValueName)
	}

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
			return err
		}
		processValueNames = append(processValueNames, processValueName)
	}

	// Check if length of processValueNameTable is equal to processValueNames
	var processValueNameTableLength int
	err = db.QueryRow("SELECT COUNT(*) FROM processValueNameTable").Scan(&processValueNameTableLength)
	if err != nil {
		return err
	}

	if processValueNameTableLength != len(processValueNames) {

		zap.S().Infof("Inserting %d process value names", len(processValueNames))
		// insert process value names set and get there ids

		var prepCopy *sql.Stmt
		prepCopy, err = db.Prepare(pq.CopyIn("tmp_processValueNameTable", "name"))
		if err != nil {
			zap.S().Errorf("Error while preparing copy: %v", err)
			return err
		}

		var prepSelectAll *sql.Stmt
		prepSelectAll, err = db.Prepare("SELECT id, name FROM processValueNameTable")

		defer func(stmt1, stmt2 *sql.Stmt) {
			if err := stmt1.Close(); err != nil {
				zap.S().Errorf("Error while closing prepared statement: %v", err)
			}
			if err := stmt2.Close(); err != nil {
				zap.S().Errorf("Error while closing prepared statement: %v", err)
			}
		}(prepCopy, prepSelectAll)

		// deduplicate processValueNames
		zap.S().Infof("Deduplicating process value names")
		processValueNames = deduplicate(processValueNames)

		// Shuffling
		for i := range processValueNames {
			j := rand.Intn(i + 1)
			processValueNames[i], processValueNames[j] = processValueNames[j], processValueNames[i]
		}

		zap.S().Infof("Inserting process value names into work channel")
		var cvChan = make(chan string, len(processValueNames))
		for i := 0; i < len(processValueNames); i++ {
			cvChan <- processValueNames[i]
		}

		var wg sync.WaitGroup
		var threads = 90
		wg.Add(threads)
		zap.S().Infof("Starting %d threads", threads)
		for i := 0; i < threads; i++ {
			go concurrentInsertScan(cvChan, prepCopy, &wg)
		}
		wg.Wait()
		zap.S().Infof("All threads finished")

		// Copy tmp table to processValueNameTable
		zap.S().Infof("Copying tmp table to processValueNameTable")
		_, err = db.Exec("INSERT INTO processValueNameTable (name) (SELECT name FROM tmp_processValueNameTable) ON CONFLICT do nothing")

	}
	zap.S().Infof("Getting all process value names")
	// Get all process value names and there ids and put them into pvNMap
	rows, err = db.Query("SELECT * FROM processValueNameTable")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		var name string
		err = rows.Scan(&id, &name)
		if err != nil {
			return err
		}
		pvNMapRWMutex.Lock()
		pvNMap[name] = id
		pvNMapRWMutex.Unlock()
	}
	return nil

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
					// cast err to sql error
					sqlErr, ok := err.(*pq.Error)
					if !ok {
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
				if current := concurrentInsertScanCounterAtomic.Load(); current%1000 == 0 {
					zap.S().Infof("Inserted %d process value names", current)
				}
			} else {
				zap.S().Infof("Channel closed!")
				s.Done()
				return
			}
		default:
			zap.S().Infof("Channel empty!, returining")
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
		"componenttable_workCellId_fkey",
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
	err = database.AddFKConstraint("ordertable_workCellId_fkey", "ordertable", "workCellId", "workCellTable", "id", db)
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
	return migrateProcessXTable("processvaluetable", db)
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
		"valuename",
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
