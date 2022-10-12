package controllers

import (
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/services"
	"net/http"
)

func GetWorkCellsHandler(c *gin.Context) {
	var request models.GetWorkCellsRequest
	var workCells models.GetWorkCellsResponse

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
	workCells, err = services.GetWorkCells(request.EnterpriseName, request.SiteName, request.AreaName, request.ProductionLineName)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "No work cells found"})
	} else if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, workCells)
}
