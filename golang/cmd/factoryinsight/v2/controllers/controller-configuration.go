package controllers

import (
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/services"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"net/http"
)

// GetConfigurationHandler handles a GET request to /api/v2/{enterprise}/configuration
func GetConfigurationHandler(c *gin.Context) {
	var request models.GetSitesRequest
	var configuration datamodel.EnterpriseConfiguration

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
	configuration, err = services.GetEnterpriseConfiguration(request.EnterpriseName)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "No configuration found"})
	} else if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, configuration)
}
