package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/services"
	"net/http"
)

type getTagGroupsRequest struct {
	EnterpriseName     string `json:"enterpriseName"`
	SiteName           string `json:"siteName"`
	AreaName           string `json:"areaName"`
	ProductionLineName string `json:"productionLineName"`
	WorkCellName       string `json:"workCellName"`
}

func GetTagGroupsHandler(c *gin.Context) {
	var request getTagGroupsRequest
	var tagGroups models.GetTagGroupsResponse

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
