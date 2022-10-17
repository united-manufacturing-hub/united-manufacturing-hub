package services

import (
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

	var stateExists bool
	stateExists, err = GetStateExists(workCellId)
	if err != nil {
		return
	}
	if stateExists { // TODO: Check if this is correct
		dataFormats = []string{models.TagsDataFormat, models.KpisDataFormat, models.TablesDataFormat}
	} else {
		dataFormats = []string{""}
	}

	return
}
