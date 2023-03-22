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

func GetAllWorkCellIds() (workCellIds []uint32, err error) {
	workCellIds = make([]uint32, 0)

	sqlStatement := `SELECT id FROM assetTable;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement)

	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Warnf("[GetAllWorkCellIds] No work cells found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	defer rows.Close()

	for rows.Next() {
		var assetId uint32
		err = rows.Scan(&assetId)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		workCellIds = append(workCellIds, assetId)
	}

	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}
	return
}
