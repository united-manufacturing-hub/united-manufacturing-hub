// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package services

/*


import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/models"
)

func GetTableTypes(
	enterpriseName string,
	siteName string,
	areaName string,
	productionLineName string,
	workCellName string) (tables models.GetTableTypesResponse, err error) {

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
	if err != nil {
		return
	}

	sqlStatement := `SELECT EXISTS(SELECT 1 FROM stateTable WHERE WorkCellId = $1)`

	var stateExists bool
	err = db.QueryRow(sqlStatement, workCellId).Scan(&stateExists)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	if stateExists {
		tables.Tables = append(tables.Tables, models.TableType{Id: 0, Name: models.JobTable})
		tables.Tables = append(tables.Tables, models.TableType{Id: 2, Name: models.ProductsTable})
		tables.Tables = append(tables.Tables, models.TableType{Id: 3, Name: models.ProductTypesTable})
	}

	return
}

func ProcessJobsTableRequest(c *gin.Context, request models.GetTableDataRequest) {
	// TODO adapt this to the new data model

	// ### store request values in proper variables ###
	enterprise := request.EnterpriseName
	site := request.SiteName
	area := request.AreaName
	productLine := request.ProductionLineName
	workCell := request.WorkCellName

	// ### parse query ###
	var getJobTableRequest models.GetJobTableRequest
	var err error

	err = c.BindUri(&getJobTableRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetch data from database
	zap.S().Debugf(
		"Fetching order table for customer %s, location %s, asset %s, value: %v",
		request.Customer,
		request.Location,
		request.Asset,
		request.Value)

	// customer configuration
	zap.S().Debugf("GetEnterpriseConfiguration")
	configuration, err := GetEnterpriseConfiguration(request.Customer)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	zap.S().Debugf("GetAssetID")
	assetID, err := GetAssetID(request.Customer, request.Location, request.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	zap.S().Debugf("GetOrdersRaw")
	rawOrders, err := GetOrdersRaw(
		request.Customer,
		request.Location,
		request.Asset,
		getJobTableRequest.From,
		getJobTableRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for actual units calculation
	zap.S().Debugf("GetCountsRaw")
	countSlice, err := GetCountsRaw(
		request.Customer,
		request.Location,
		request.Asset,
		getJobTableRequest.From,
		getJobTableRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// raw states from database
	zap.S().Debugf("GetStatesRaw")
	rawStates, err := GetStatesRaw(
		request.Customer,
		request.Location,
		request.Asset,
		getJobTableRequest.From,
		getJobTableRequest.To,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	zap.S().Debugf("GetShiftsRaw")
	rawShifts, err := GetShiftsRaw(
		request.Customer,
		request.Location,
		request.Asset,
		getJobTableRequest.From,
		getJobTableRequest.To,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// TODO: #98 Return timestamps in RFC3339 in /orderTable

	// Process data
	// zap.S().Debugf("calculateOrderInformation: rawOrders: %v, countSlice: %v, assetID: %v, rawStates: %v, rawShifts: %v, configuration: %v, Location: %v, Asset: %v", rawOrders, countSlice, assetID, rawStates, rawShifts, configuration, request.Location, request.Asset)
	data, err := calculateOrderInformation(
		rawOrders,
		countSlice,
		assetID,
		rawStates,
		rawShifts,
		configuration,
		request.Location,
		request.Asset)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
	* /
}

func ProcessProductsTableRequest(c *gin.Context, request models.GetTableDataRequest) {
	// TODO adapt this to the new data model

	// ### store request values in proper variables ###
	enterprise := request.EnterpriseName
	site := request.SiteName
	area := request.AreaName
	productLine := request.ProductionLineName
	workCell := request.WorkCellName

	// ### parse query ###
	var getProductsTableRequest models.GetProductsTableRequest
	var err error

	err = c.BindUri(&getProductsTableRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// TODO: #99 Return timestamps in RFC3339 in /products

	// Fetching from the database
	products, err := GetUniqueProducts(
		request.Customer,
		request.Location,
		request.Asset,
		getProductsTableRequest.From,
		getProductsTableRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, products)
	* /
}

func ProcessProductTypesTableRequest(c *gin.Context, request models.GetTableDataRequest) {
	// TODO adapt this to the new data model

	// ### store request values in proper variables ###
	enterprise := request.EnterpriseName
	site := request.SiteName
	area := request.AreaName
	productLine := request.ProductionLineName
	workCell := request.WorkCellName

	// ### parse query ###
	var getProductTypesTableRequest models.GetProductTypesTableRequest
	var err error

	err = c.BindUri(&getProductTypesTableRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	productTypes, err := GetProductTypes(
		request.Customer,
		request.Location,
		request.Asset,
		getProductTypesTableRequest.From,
		getProductTypesTableRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, productTypes)
	* /
}
*/
