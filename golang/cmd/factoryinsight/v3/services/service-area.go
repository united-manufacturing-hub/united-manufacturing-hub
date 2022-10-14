package services

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
