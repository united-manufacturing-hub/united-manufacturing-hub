package controllers
/*


import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/services"
	"net/http"
)

func GetDataFormatHandler(c *gin.Context) {
	var request models.GetDataFormatRequest

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
	dataFormats, err := services.GetDataFormats(
		request.EnterpriseName,
		request.SiteName,
		request.AreaName,
		request.ProductionLineName,
		request.WorkCellName)
	// TODO: Better error handling. Check if the error is a database error or a not found error (dataFormats is empty)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, dataFormats)
}
*/
