package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

func GetStandardTags() (tags models.GetTagsResponse, err error) {
	zap.S().Infof("[GetTags] Getting standard tags")

	for id, tag := range internal.StandardTags {
		tags.Tags = append(tags.Tags, models.Tag{Id: id, Name: tag})
	}
	return
}

func GetCustomTags(
	enterpriseName string,
	siteName string,
	areaName string,
	productionLineName string,
	workCellName string,
	tagGroupName string,
) (tags models.GetTagsResponse, err error) {
	zap.S().Infof(
		"[GetTags] Getting custom tags for enterprise %s, site %s, area %s, production line %s, work cell %s and tag group %s",
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
		tagGroupName,
	)
	// TODO: Implement GetCustomTags
	return
}
