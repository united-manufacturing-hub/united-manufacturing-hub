package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

func GetTagGroups(
	enterpriseName string,
	siteName string,
	areaName string,
	productionLineName string,
	workCellName string,
) (tagGroups models.GetTagGroupsResponse, err error) {
	zap.S().Infof(
		"[GetTagGroups] Getting tag groups for enterprise %s, site %s, area %s, production line %s and work cell %s",
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
	)

	tagGroups.TagGroups = append(tagGroups.TagGroups, models.TagGroup{Id: 1, Name: "standard"})
	tagGroups.TagGroups = append(tagGroups.TagGroups, models.TagGroup{Id: 2, Name: "custom"})
	tagGroups.TagGroups = append(tagGroups.TagGroups, models.TagGroup{Id: 3, Name: "raw"})
	err = nil
	return
}

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
