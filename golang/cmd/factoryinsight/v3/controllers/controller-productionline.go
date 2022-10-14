package controllers

import (
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/services"
	"net/http"
)

func GetProductionLinesHandler(c *gin.Context) {
	var request models.GetProductionLineRequest
	var productionLines models.GetProductionLinesResponse

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
	productionLines, err = services.GetProductionLines(request.EnterpriseName, request.SiteName, request.AreaName)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "No production lines found"})
	} else if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, productionLines)
}
