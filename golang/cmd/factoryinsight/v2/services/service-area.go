package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"go.uber.org/zap"
)

// GetAreas returns all areas of a site
func GetAreas(enterpriseName string, siteName string) (areas []string, err error) {
	zap.S().Infof("[GetAreas] Getting areas for enterprise %s and site %s", enterpriseName, siteName)

	areas = []string{models.MockDefaultArea}

	return
}
