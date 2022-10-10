package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/services"
	"net/http"
)

type getTagsRequest struct {
	EnterpriseName     string `json:"enterpriseName"`
	SiteName           string `json:"siteName"`
	AreaName           string `json:"areaName"`
	ProductionLineName string `json:"productionLineName"`
	WorkCellName       string `json:"workCellName"`
	TagGroupName       string `json:"tagGroupName"`
}

func GetTagsHandler(c *gin.Context) {
	var request getTagsRequest
	var tags []models.GetTagsResponse

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
	case "standard":
		tags, err = services.GetStandardTags()
	case "custom":
		tags, err = services.GetCustomTags(request.EnterpriseName, request.SiteName, request.AreaName, request.ProductionLineName, request.WorkCellName, request.TagGroupName)
	case "raw": // TODO: Properly handle raw tags
		return
	default:
		helpers.HandleInvalidInputError(c, err)
		return
	}

	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, tags)
}
