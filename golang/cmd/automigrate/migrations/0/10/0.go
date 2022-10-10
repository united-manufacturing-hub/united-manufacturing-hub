package _10

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
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
		return err
	}
	return nil
}

func checkIfColumnExists(colName, tableName string, db *sql.DB) (bool, error) {
	var assetIdExists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = " + tableName + " AND column_name = '" + colName + "')").Scan(&assetIdExists)
	if err != nil {
		return false, err
	}
	return assetIdExists, nil
}

func migrateComponentTable(db *sql.DB, idMap map[int]int) error {
	// Check if asset_id column exists
	assetIdExists, err := checkIfColumnExists("asset_id", "componentTable", db)
	if err != nil {
		return err
	}
	if !assetIdExists {
		zap.S().Info("Asset_id column does not exist in componentTable. Skipping migration")
		return nil
	}

	var dropConstraintCommand = `
	ALTER TABLE componentTable DROP CONSTRAINT componenttable_asset_id_fkey;
	`
	_, err = db.Exec(dropConstraintCommand)
	if err != nil {
		return err
	}

	var insertAddColumnCommand = `
	ALTER TABLE componentTable ADD COLUMN workCellId int;
	ALTER TABLE componenttable ADD CONSTRAINT componenttable_workCellId_componentname_key UNIQUE (workCellId, componentname);
	ALTER TABLE componenttable ADD CONSTRAINT componenttable_workCellId_fkey FOREIGN KEY (workCellId) REFERENCES workCellTable (id);
`
	_, err = db.Exec(insertAddColumnCommand)
	if err != nil {
		return err
	}

	// Migrate data from asset_id to workCellId
	for oldId, newId := range idMap {
		_, err = db.Exec("UPDATE componentTable SET workCellId = $1 WHERE asset_id = $2", newId, oldId)
		if err != nil {
			return err
		}
	}

	// Drop asset_id column
	var dropColumnCommand = `
	ALTER TABLE componenttable DROP CONSTRAINT componenttable_asset_id_componentname_key;
	ALTER TABLE componentTable DROP COLUMN asset_id;
	`

	_, err = db.Exec(dropColumnCommand)
	if err != nil {
		return err
	}

	return nil
}
