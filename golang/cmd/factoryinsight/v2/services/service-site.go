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

// GetSites returns all sites of an enterprise
func GetSites(enterpriseName string) (sites []string, err error) {
	zap.S().Infof("[GetSites] Getting sites for enterprise %s", enterpriseName)

	sqlStatement := `SELECT distinct(location) FROM assetTable WHERE customer=$1;`

	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, enterpriseName)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	defer rows.Close()

	for rows.Next() {
		var location string
		err = rows.Scan(&location)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}
		sites = append(sites, location)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	return
}
