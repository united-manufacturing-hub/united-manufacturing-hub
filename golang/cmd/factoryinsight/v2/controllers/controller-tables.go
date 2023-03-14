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

package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/services"
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
	tables, err := services.GetTableTypes(request.EnterpriseName, request.SiteName, request.WorkCellName)
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
	case models.JobsTable:
		services.ProcessJobsTableRequest(c, request)
	case models.ProductsTable:
		services.ProcessProductsTableRequest(c, request)
	case models.ProductTypesTable:
		services.ProcessProductTypesTableRequest(c, request)
	case models.AvailabilityHistogramTable:
		services.ProcessAvailabilityHistogramTableRequest(c, request)
	case models.AvailabilityTotalTable:
		services.ProcessAvailabilityTotalTableRequest(c, request)
	case models.PerformanceTable:
		services.ProcessPerformanceTableRequest(c, request)
	case models.QualityTable:
		services.ProcessQualityTableRequest(c, request)
	default:
		helpers.HandleTypeNotFound(c, request.TableType)
		return
	}
}
