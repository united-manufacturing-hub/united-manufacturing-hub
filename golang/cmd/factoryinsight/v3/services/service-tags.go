// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package services

/*


import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/models"
	"go.uber.org/zap"
	"net/http"
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

func ProcessJobTagRequest(c *gin.Context, request models.GetTagsDataRequest) {
	// TODO adapt this to the new data model

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### parse query ###
	var getJobTagRequest models.GetJobTagRequest

	err = c.BindQuery(&getJobTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// TODO: #97 Return timestamps in RFC3339 in /orderTimeline

	// Process data
	data, err := GetOrdersTimeline(workCellId, getJobTagRequest.From, getJobTagRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func ProcessOutputTagRequest(c *gin.Context, request models.GetTagsDataRequest) {
	// TODO adapt this to the new data model

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### parse query ###
	var getCountTagRequest models.GetCountTagRequest
	// var counts ???

	err = c.BindQuery(&getCountTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	// TODO: #88 Return timestamps in RFC3339 in /counts
	counts, err = GetCounts(workCellId, getCountTagRequest.From, getCountTagRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
	* /
}

func ProcessShiftsTagRequest(c *gin.Context, request models.GetTagsDataRequest) {
	// TODO adapt this to the new data model

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### parse query ###
	var getShiftsTagRequest models.GetShiftsTagRequest

	err = c.BindQuery(&getShiftsTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	shifts, err := GetShifts(enterpriseName, workCellId, getShiftsTagRequest.From, getShiftsTagRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, shifts)
}

// ProcessStateTagRequest is responsible for fetchinf all required data and calculating states over time.
// The result is usually visualized in "DiscretePanel" in Grafana
func ProcessStateTagRequest(c *gin.Context, request models.GetTagsDataRequest) {
	// TODO adapt this to the new data model

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	// ### parse query ###
	var getStateTagRequest models.GetStateTagRequest
	var err error

	err = c.BindQuery(&getStateTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getStateTagRequest.From
	to := getStateTagRequest.To
	keepStatesInteger := getStateTagRequest.KeepStatesInteger

	// ### fetch necessary data from database ###

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetEnterpriseConfiguration(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// raw states from database
	rawStates, err := GetStatesRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := processStatesOptimized(
		workCellId,
		rawStates,
		rawShifts,
		countSlice,
		orderArray,
		from,
		to,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### create JSON ###
	var data datamodel.DataResponseAny
	// TODO: adapt JSONColumnName to new data model
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "state"
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: #90 Return timestamps in RFC3339 in /state

	// Loop through all datapoints
	for _, dataPoint := range processedStates {
		if keepStatesInteger {
			fullRow := []interface{}{
				dataPoint.State,
				float64(dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
			data.Datapoints = append(data.Datapoints, fullRow)
		} else {
			fullRow := []interface{}{
				repository.ConvertStateToString(dataPoint.State, configuration),
				float64(dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
			data.Datapoints = append(data.Datapoints, fullRow)
		}
	}

	c.JSON(http.StatusOK, data)
	* /
}

func ProcessThroughputTagRequest(c *gin.Context, request models.GetTagsDataRequest) {
	// TODO adapt this to the new data model

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### parse query ###
	var getThroughputTagRequest models.GetThroughputTagRequest
	// var counts ???

	err = c.BindQuery(&getThroughputTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getThroughputTagRequest.From
	to := getThroughputTagRequest.To
	aggregationInterval := getThroughputTagRequest.AggregationInterval

	// Fetching from the database
	counts, err = GetProductionSpeed(workCellId, from, to, aggregationInterval)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
	* /
}
*/
