package _10

import (
	"database/sql"
	"errors"
	"go.uber.org/zap"
)

func V0x10x0(db *sql.DB) error {
	zap.S().Infof("Applying migration 0.10.0")

	err := createNewTables(db)
	if err != nil {
		zap.S().Fatalf("Error while creating new tables: %v", err)
	}

	return errors.New("not implemented")
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

    CREATE INDEX ON processValueNameTable (name);
`

	_, err := db.Exec(creationCommand)
}
