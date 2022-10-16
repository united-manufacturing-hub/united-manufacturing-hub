package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"go.uber.org/zap"
)

// GetProductionLines returns all production lines for a given area
func GetProductionLines(
	enterpriseName string,
	siteName string,
	areaName string,
) (productionLines []string, err error) {

	zap.S().Infof(
		"[GetProductionLines] Getting production lines for enterprise %s, site %s and area %s",
		enterpriseName,
		siteName,
		areaName,
	)

	productionLines = []string{models.MockProductionLine1, models.MockProductionLine2, models.MockProductionLine3}

	return
}
