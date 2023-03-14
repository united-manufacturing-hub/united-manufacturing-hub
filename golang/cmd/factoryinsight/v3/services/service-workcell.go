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
	"database/sql"
	"errors"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/models"
	"go.uber.org/zap"
)

func GetWorkCells(
	entrerpriseName string,
	siteName string,
	areaName string,
	productionLineName string,
) (workCells models.GetWorkCellsResponse, err error) {
	zap.S().Infof(
		"[GetWorkCells] Getting work cells for enterprise %s, site %s, area %s and production line %s",
		entrerpriseName,
		siteName,
		areaName,
		productionLineName,
	)

	sqlStatement := `SELECT id, name FROM workCellTable WHERE enterpriseName = $1 AND siteName = $2 AND areaName = $3 AND productionLineName = $4`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, entrerpriseName, siteName, areaName, productionLineName)
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
		var workCell models.WorkCell
		err = rows.Scan(&workCell.Id, &workCell.Name)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		workCells.WorkCells = append(workCells.WorkCells, workCell)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}
*/
