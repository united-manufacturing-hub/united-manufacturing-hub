package services

import (
	"database/sql"
	"errors"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"go.uber.org/zap"
)

//func init() {
//	db = database.Db
//	Mutex = database.Mutex
//	GracefulShutdownChannel = database.GracefulShutdownChannel
//}

// GetSites returns all sites of an enterprise
func GetSites(enterpriseName string) (sites models.GetSitesResponse, err error) {
	zap.S().Infof("[GetSites] Getting sites for enterprise %s", enterpriseName)

	sqlStatement := `SELECT id, name FROM siteTable WHERE enterpriseName = $1`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, enterpriseName)
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Warnf("[GetSites] No sites found for enterprise %s", enterpriseName)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var site models.Site
		err = rows.Scan(&site.Id, &site.Name)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		sites.Sites = append(sites.Sites, site)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}
