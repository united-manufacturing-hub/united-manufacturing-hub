package services

import (
	"database/sql"
	"errors"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"go.uber.org/zap"
)

// GetProductionLines returns all production lines for a given area
func GetProductionLines(enterpriseName string, siteName string, areaName string) (productionLines []models.GetProductionLinesResponse, err error) {
	zap.S().Infof("[GetProductionLines] Getting production lines for enterprise %s, site %s and area %s", enterpriseName, siteName, areaName)

	sqlStatement := `SELECT id, name FROM productionLineTable WHERE enterpriseName = $1 AND siteName = $2 AND areaName = $3`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, enterpriseName, siteName, areaName)
	if errors.Is(err, sql.ErrNoRows) {
		zap.S().Warnf("[GetProductionLines] No production lines found for enterprise %s, site %s and area %s", enterpriseName, siteName, areaName)
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var productionLine models.ProductionLine
		err = rows.Scan(&productionLine.Id, &productionLine.Name)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		productionLines = append(productionLines, models.GetProductionLinesResponse{ProductionLines: []models.ProductionLine{productionLine}})
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}
