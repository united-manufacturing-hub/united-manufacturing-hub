package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"go.uber.org/zap"
)

func GetTagGroups(enterpriseName string, siteName string, areaName string, productionLineName string, workCellName string) (tagGroups []models.GetTagGroupsResponse, err error) {
	zap.S().Infof("[GetTagGroups] Getting tag groups for enterprise %s, site %s, area %s, production line %s and work cell %s", enterpriseName, siteName, areaName, productionLineName, workCellName)

	tagGroups = append(tagGroups, models.GetTagGroupsResponse{TagGroups: []models.TagGroup{{Id: 1, Name: "standard"}, {Id: 2, Name: "custom"}, {Id: 3, Name: "raw"}}})
	err = nil
	return
}
