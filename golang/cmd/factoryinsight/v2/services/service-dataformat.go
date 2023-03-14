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

	//dataFormats = []string{models.TagsDataFormat, models.KpisDataFormat, models.TablesDataFormat}
	dataFormats = make([]string, 0)

	tagGroups, err := GetTagGroups(enterpriseName, siteName, areaName, productionLineName, workCellName)
	if err != nil {
		return nil, err
	}

	if len(tagGroups) > 0 {
		dataFormats = append(dataFormats, models.TagsDataFormat)
	}

	kpis, err := GetKpisMethods(enterpriseName, siteName, workCellName)
	if err != nil {
		return nil, err
	}

	if len(kpis.Kpis) > 0 {
		dataFormats = append(dataFormats, models.KpisDataFormat)
	}

	tables, err := GetTableTypes(enterpriseName, siteName, workCellName)
	if err != nil {
		return nil, err
	}

	if len(tables.Tables) > 0 {
		dataFormats = append(dataFormats, models.TablesDataFormat)
	}

	return
}
