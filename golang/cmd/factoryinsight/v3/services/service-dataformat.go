// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package services

/*


import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/models"
	"go.uber.org/zap"
)

func GetDataFormats(
	enterpriseName string,
	siteName string,
	areaName string,
	productionLineName string,
	workCellName string,
) (dataFormats models.GetDataFormatResponse, err error) {
	zap.S().Infof(
		"[GetWorkCells] Getting data formats for enterprise %s, site %s, area %s, production line %s and work cell %s",
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
	)

	var enterpriseId, siteId, areaId, productionLineId, workCellId uint32
	enterpriseId, err = GetEnterpriseId(enterpriseName)
	if err != nil {
		return
	}
	siteId, err = GetSiteId(enterpriseId, siteName)
	if err != nil {
		return
	}
	areaId, err = GetAreaId(siteId, areaName)
	if err != nil {
		return
	}
	productionLineId, err = GetProductionLineId(areaId, productionLineName)
	if err != nil {
		return
	}
	workCellId, err = GetWorkCellId(productionLineId, workCellName)
	if err != nil {
		return
	}

	sqlStatement := `SELECT EXISTS(SELECT 1 FROM stateTable WHERE workCellId = $1)`

	var stateExists bool
	err = db.QueryRow(sqlStatement, workCellId).Scan(&stateExists)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}
	if stateExists { // TODO: Check if this is correct
		dataFormats.DataFormats = append(
			dataFormats.DataFormats, models.DataFormat{
				Id:   1,
				Name: models.TagsDataFormat,
			})
		dataFormats.DataFormats = append(
			dataFormats.DataFormats, models.DataFormat{
				Id:   2,
				Name: models.KpisDataFormat,
			})
		dataFormats.DataFormats = append(
			dataFormats.DataFormats, models.DataFormat{
				Id:   3,
				Name: models.ListsDataFormat,
			})
	}

	return
}
*/
