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

//func init() {
//	db = database.Db
//	Mutex = database.Mutex
//	GracefulShutdownChannel = database.GracefulShutdownChannel
//}

// GetAreas returns all areas of a site
func GetAreas(enterpriseName string, siteName string) (areas models.GetAreasResponse, err error) {
	zap.S().Infof("[GetAreas] Getting areas for enterprise %s and site %s", enterpriseName, siteName)

	sqlStatement := `SELECT id, name FROM areaTable WHERE enterpriseName = $1 AND siteName = $2`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, enterpriseName, siteName)
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Warnf("[GetAreas] No areas found for enterprise %s and site %s", enterpriseName, siteName)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var area models.Area
		err = rows.Scan(&area.Id, &area.Name)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		areas.Areas = append(areas.Areas, area)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}
*/
