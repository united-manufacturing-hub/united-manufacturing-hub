package services

import (
	"database/sql"
	"errors"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"go.uber.org/zap"
)

func GetWorkCells(
	entrerpriseName string,
	siteName string,
	areaName string,
	productionLineName string,
) (workCells []string, err error) {
	zap.S().Infof(
		"[GetWorkCells] Getting work cells for enterprise %s, site %s, area %s and production line %s",
		entrerpriseName,
		siteName,
		areaName,
		productionLineName,
	)

	sqlStatement := `SELECT distinct(assetID) FROM assetTable WHERE customer=$1 AND location=$2;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, entrerpriseName, siteName)
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Warnf(
			"[GetWorkCells] No work cells found for enterprise %s, site %s, area %s and production line %s",
			entrerpriseName,
			siteName,
			areaName,
			productionLineName)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var asset string
		err = rows.Scan(&asset)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		workCells = append(workCells, asset)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}
