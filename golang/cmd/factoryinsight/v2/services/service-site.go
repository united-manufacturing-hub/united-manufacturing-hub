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
	rows, err = db.Query(sqlStatement, enterpriseName)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
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
