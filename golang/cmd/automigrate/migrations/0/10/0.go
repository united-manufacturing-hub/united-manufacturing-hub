package _10

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/automigrate/database"
	"go.uber.org/zap"
)

func V0x10x0(db *sql.DB) error {
	zap.S().Infof("Applying migration 0.10.0")

	zap.S().Infof("Creating new tables")
	err := createNewTables(db)
	if err != nil {
		zap.S().Fatalf("Error while creating new tables: %v", err)
	}
	zap.S().Infof("Created new tables")

	zap.S().Infof("Migrating asset table")
	var assetIdMap map[int]int
	assetIdMap, err = migrateAssetTable(db)
	if err != nil {
		zap.S().Fatalf("Error while migrating asset table: %v", err)
	}
	zap.S().Infof("Migrated asset table")

	zap.S().Infof("Migrating other tables to new ids")
	err = migrateOtherTables(db, assetIdMap)
	if err != nil {
		zap.S().Fatalf("Error while migrating other tables: %v", err)
	}
	zap.S().Infof("Migrated other tables to new ids")

	return errors.New("not implemented")
}

type AssetTable struct {
	assetid  string
	location string
	customer string
	shaSum   string
	id       int
}

func migrateAssetTable(db *sql.DB) (map[int]int, error) {
	//assetTable.customer -> enterpriseTable.name
	//assetTable.location -> siteTable.name
	//assetTable.assetid -> workCellTable.name
	//areaTable.name -> defaultArea + SHA256(assetTable.customer, assetTable.location)
	//productionLineTable.name -> defaultProductionLine + SHA256(assetTable.customer, assetTable.location)

	// Get all assets
	rows, err := db.Query("SELECT * FROM assetTable")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []AssetTable

	for rows.Next() {
		var row AssetTable
		err = rows.Scan(&row.id, &row.assetid, &row.location, &row.customer)
		if err != nil {
			return nil, err
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
	var assetIdMap = make(map[int]int)

	// Insert all assets into new tables
	for _, asset := range assets {
		// Insert enterprise
		_, err = db.Exec("INSERT INTO enterpriseTable (name) VALUES ($1) ON CONFLICT do nothing ", asset.customer)
		if err != nil {
			zap.S().Errorf("Error while inserting enterprise: %v", err)
			return nil, err
		}

		// Insert site
		_, err = db.Exec(
			"INSERT INTO siteTable (name, enterpriseId) VALUES ($1, (SELECT id FROM enterpriseTable WHERE name = $2)) ON CONFLICT do nothing ",
			asset.location,
			asset.customer)
		if err != nil {
			zap.S().Errorf("Error while inserting site: %v", err)
			return nil, err
		}

		// Insert area
		_, err = db.Exec(
			"INSERT INTO areaTable (name, siteId) VALUES ($1, (SELECT id FROM siteTable WHERE name = $2 AND enterpriseid = (SELECT id FROM enterprisetable WHERE name = $3))) ON CONFLICT do nothing ",
			"defaultArea"+asset.shaSum,
			asset.location,
			asset.customer)
		if err != nil {
			zap.S().Errorf("Error while inserting area: %v", err)
			return nil, err
		}

		// Insert production line
		_, err = db.Exec(
			"INSERT INTO productionLineTable (name, areaId) VALUES ($1, (SELECT id FROM areaTable WHERE name = $2)) ON CONFLICT do nothing ",
			"defaultProductionLine"+asset.shaSum,
			"defaultArea"+asset.shaSum)
		if err != nil {
			zap.S().Errorf("Error while inserting production line: %v", err)
			return nil, err
		}

		// Insert work cell
		_, err = db.Exec(
			"INSERT INTO workCellTable (name, productionLineId) VALUES ($1, (SELECT id FROM productionLineTable WHERE name = $2)) ON CONFLICT do nothing ",
			asset.assetid,
			"defaultProductionLine"+asset.shaSum)
		if err != nil {
			zap.S().Errorf("Error while inserting work cell: %v", err)
			return nil, err
		}

		// Get new asset id
		var newAssetId int
		err = db.QueryRow(
			"SELECT id FROM workCellTable WHERE name = $1 AND productionLineId = (SELECT id FROM productionLineTable WHERE name = $2) ",
			asset.assetid,
			"defaultProductionLine"+asset.shaSum).Scan(&newAssetId)

		if err != nil {
			zap.S().Errorf("Error while getting new asset id: %v", err)
			return nil, err
		}
		// Add to map
		assetIdMap[asset.id] = newAssetId
	}

	return assetIdMap, nil
}

func createNewTables(db *sql.DB) error {
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

func migrateOtherTables(db *sql.DB, idMap map[int]int) error {
	err := migrateComponentTable(db, idMap)
	if err != nil {
		zap.S().Errorf("Error while migrating component table: %v", err)
		return err
	}
	err = migrateCountTable(db, idMap)
	if err != nil {
		zap.S().Errorf("Error while migrating count table: %v", err)
		return err
	}
	return nil
}

func AssetIdToWorkCellIdConverter(tableName string, db *sql.DB, idMap map[int]int) error {
	var err error
	for oldId, newId := range idMap {
		_, err = db.Exec(fmt.Sprintf("UPDATE %s SET workCellId = %d WHERE asset_id = %d", tableName, newId, oldId))
		if err != nil {
			return err
		}
	}
	return nil
}

func migrateComponentTable(db *sql.DB, idMap map[int]int) error {
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

	err = AssetIdToWorkCellIdConverter("componenttable", db, idMap)
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

func migrateCountTable(db *sql.DB, idMap map[int]int) error {
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

	err = AssetIdToWorkCellIdConverter("counttable", db, idMap)
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
