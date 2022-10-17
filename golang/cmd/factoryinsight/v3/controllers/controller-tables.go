package controllers

/*


import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/services"
	"net/http"
)

func GetTableTypesHandler(c *gin.Context) {
	var request models.GetTableTypesRequest

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
	tables, err := services.GetTableTypes(
		request.EnterpriseName,
		request.SiteName,
		request.AreaName,
		request.ProductionLineName,
		request.WorkCellName)
	// TODO: Better error handling. Check if the error is a database error or a not found error (tables is empty)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, tables)
}

func GetTableDataHandler(c *gin.Context) {
	var request models.GetTableDataRequest

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

	switch request.TableType {
	case models.JobTable:
		services.ProcessJobsTableRequest(c, request)
	case models.ProductsTable:
		services.ProcessProductsTableRequest(c, request)
	case models.ProductTypesTable:
		services.ProcessProductTypesTableRequest(c, request)
	default:
		helpers.HandleInvalidInputError(c, err)
		return
	}
}
*/
