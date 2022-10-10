package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/services"
	"net/http"
)

type getMethodsRequest struct {
	EnterpriseName     string `json:"enterpriseName"`
	SiteName           string `json:"siteName"`
	AreaName           string `json:"areaName"`
	ProductionLineName string `json:"productionLineName"`
	WorkCellName       string `json:"workCellName"`
	TagGroupName       string `json:"tagGroupName"`
	TagName            string `json:"tagName"`
}

func GetMethodsHandler(c *gin.Context) {
	var request getMethodsRequest
	var methods []string

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
		methods, err = services.GetStandardMethods()
	case "custom":
		return // TODO: Implement custom methods
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

	c.JSON(http.StatusOK, methods)
}
