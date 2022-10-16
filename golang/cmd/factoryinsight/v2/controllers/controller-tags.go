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
	tagGroups, err = services.GetTagGroups(request.EnterpriseName, request.SiteName, request.AreaName, request.ProductionLineName, request.WorkCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, tagGroups)
}

func GetTagsHandler(c *gin.Context) {
	var request models.GetTagsRequest
	var tags []string
	var grouping = make(map[string][]string)
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

	switch request.TagGroupName {
	case models.CustomTagGroup:
		tags, err = services.GetStandardTags()
		var response models.GetTagsResponse[[]string]
		response.Tags = tags
	case models.StandardTagGroup:
		grouping, err = services.GetCustomTags(request.WorkCellName)
		var response models.GetTagsResponse[map[string][]string]
		response.Tags = grouping
	default:
		helpers.HandleInvalidInputError(c, err)
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
			helpers.HandleInvalidInputError(c, err)
			return
		}
	case models.CustomTagGroup:
		// TODO: Implement custom tags
	default:
		helpers.HandleInvalidInputError(c, err)
		return
	}
}
