package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"go.uber.org/zap"
)

func GetDataFormats(
	enterpriseName string,
	siteName string,
	areaName string,
	productionLineName string,
	workCellName string,
) (dataFormats []string, err error) {
	zap.S().Infof(
		"[GetWorkCells] Getting data formats for enterprise %s, site %s, area %s, production line %s and work cell %s",
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
	)

	var workCellId uint32
	workCellId, err = GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		return
	}

	sqlStatement := `SELECT EXISTS(SELECT 1 FROM stateTable WHERE asset_id = $1)`

	var stateExists bool
	err = database.Db.QueryRow(sqlStatement, workCellId).Scan(&stateExists)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}
	if stateExists { // TODO: Check if this is correct
		dataFormats = []string{models.TagsDataFormat, models.KpisDataFormat, models.TablesDataFormat}
	} else {
		dataFormats = []string{""}
	}

	return
}
