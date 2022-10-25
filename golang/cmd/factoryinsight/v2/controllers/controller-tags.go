package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/services"
	"net/http"
)

func GetTagGroupsHandler(c *gin.Context) {
	var request models.GetTagGroupsRequest
	var tagGroups []string

	err := c.BindUri(&request)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Check if the user has access to that resource
	err = helpers.CheckIfUserIsAllowed(c, request.EnterpriseName)
	if err != nil {
		return
	}

	// Fetch data from database
	tagGroups, err = services.GetTagGroups(
		request.EnterpriseName,
		request.SiteName,
		request.AreaName,
		request.ProductionLineName,
		request.WorkCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, tagGroups)
}

func GetTagsHandler(c *gin.Context) {
	var request models.GetTagsRequest
	var tags []string
	var grouping map[string][]string
	var response any

	err := c.BindUri(&request)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Check if the user has access to that resource
	err = helpers.CheckIfUserIsAllowed(c, request.EnterpriseName)
	if err != nil {
		return
	}

	var workCellId uint32
	workCellId, err = services.GetWorkCellId(request.EnterpriseName, request.SiteName, request.WorkCellName)
	if err != nil {
		helpers.HandleInternalServerError(nil, err)
		return
	}

	switch request.TagGroupName {
	case models.CustomTagGroup:
		grouping, err = services.GetCustomTags(workCellId)
		var r models.GetTagsResponse[map[string][]string]
		r.Tags = make(map[string][]string)
		r.Tags = grouping
		response = r
	case models.StandardTagGroup:
		tags, err = services.GetStandardTags(request.EnterpriseName, request.SiteName, request.WorkCellName)
		var r models.GetTagsResponse[[]string]
		r.Tags = tags
		response = r
	default:
		helpers.HandleTypeNotFound(c, request.TagGroupName)
		return
	}

	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func GetTagsDataHandler(c *gin.Context) {
	var request models.GetTagsDataRequest

	err := c.BindUri(&request)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Check if the user has access to that resource
	err = helpers.CheckIfUserIsAllowed(c, request.EnterpriseName)
	if err != nil {
		return
	}

	switch request.TagGroupName {
	case models.StandardTagGroup:
		switch request.TagName {
		case models.JobsStandardTag:
			services.ProcessJobTagRequest(c, request)
		case models.OutputStandardTag:
			services.ProcessOutputTagRequest(c, request)
		case models.ShiftsStandardTag:
			services.ProcessShiftsTagRequest(c, request)
		case models.StateStandardTag:
			services.ProcessStateTagRequest(c, request)
		case models.ThroughputStandardTag:
			services.ProcessThroughputTagRequest(c, request)

		default:
			helpers.HandleTypeNotFound(c, request.TagName)
			return
		}
	case models.CustomTagGroup:
		services.ProcessCustomTagRequest(c, request)

	default:
		helpers.HandleTypeNotFound(c, request.TagGroupName)
		return
	}
}
