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

/*


import (
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/services"
	"net/http"
)

func GetAreasHandler(c *gin.Context) {
	var request models.GetAreasRequest
	var areas models.GetAreasResponse

	err := c.BindUri(request)
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
	areas, err = services.GetAreas(request.EnterpriseName, request.SiteName)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "No sites found"})
	} else if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, areas)
}
*/
