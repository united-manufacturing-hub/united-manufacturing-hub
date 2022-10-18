package controllers

import (
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/services"
	"net/http"
)

// GetTreeStructureHandler handles a GET request to /api/v2/treeStructure
func GetTreeStructureHandler(c *gin.Context) {

	// Fetch data from database
	tree, err := services.GetTreeStructureFromCache()
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "No data found"})
	} else if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, tree)
}
