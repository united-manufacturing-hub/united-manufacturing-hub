package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
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

	tagGroups.TagGroups = append(tagGroups.TagGroups, models.TagGroup{Id: 1, Name: models.StandardTagGroup})
	tagGroups.TagGroups = append(tagGroups.TagGroups, models.TagGroup{Id: 2, Name: models.CustomTagGroup})

	return tagGroups, nil
}

func GetStandardTags() (tags models.GetTagsResponse, err error) {
	zap.S().Infof("[GetTags] Getting standard tags")

	tags.Tags = append(tags.Tags, models.Tag{Id: 1, Name: models.JobsStandardTag})
	tags.Tags = append(tags.Tags, models.Tag{Id: 2, Name: models.OutputStandardTag})
	tags.Tags = append(tags.Tags, models.Tag{Id: 3, Name: models.ShiftsStandardTag})
	tags.Tags = append(tags.Tags, models.Tag{Id: 4, Name: models.StateStandardTag})
	tags.Tags = append(tags.Tags, models.Tag{Id: 5, Name: models.ThroughputStandardTag})

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
