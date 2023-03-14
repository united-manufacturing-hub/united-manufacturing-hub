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

package helpers

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"net/http"
	"runtime/debug"
)

var InsecureNoAuth bool

func HandleInternalServerError(c *gin.Context, err error) {
	if c == nil {
		zap.S().Fatal("context is nil")
	}
	if err == nil {
		err = errors.New("unknown error")
	}

	if errors.Is(err, sql.ErrNoRows) {
		var data datamodel.DataResponseAny
		data.ColumnNames = []string{
			"error",
			"stack-trace",
		}
		data.Datapoints = [][]interface{}{
			{
				"no rows in result set",
			},
			{
				string(debug.Stack()),
			},
		}
		c.JSON(
			http.StatusNoContent, data)
		return
	}

	erx := internal.SanitizeString(err.Error())
	zap.S().Errorw(
		"Internal server error",
		"error", erx,
	)

	c.JSON(
		http.StatusInternalServerError,
		gin.H{
			"error":       erx,
			"status":      http.StatusInternalServerError,
			"message":     "The server had an internal error.",
			"stack-trace": string(debug.Stack()),
		})
}

func HandleTypeNotFound(c *gin.Context, t any) {
	if c == nil {
		zap.S().Fatal("context is nil")
	}

	zap.S().Errorw(
		"Type not found",
		t,
	)
	// Get request url from c
	route := c.FullPath()

	c.JSON(
		http.StatusNotFound,
		gin.H{
			"error":       fmt.Sprintf("Type %s not found", t),
			"status":      http.StatusNotFound,
			"message":     fmt.Sprintf("The requested type %s was not found.", t),
			"stack-trace": string(debug.Stack()),
			"route":       route,
		})
}

func HandleInvalidInputError(c *gin.Context, err error) {
	if c == nil {
		zap.S().Fatal("context is nil")
	}
	if err == nil {
		err = errors.New("unknown error")
	}
	erx := internal.SanitizeString(err.Error())
	zap.S().Errorw(
		"Invalid input error",
		"error", erx,
	)

	c.JSON(
		http.StatusBadRequest,
		gin.H{
			"error":       erx,
			"status":      http.StatusBadRequest,
			"message":     "You have provided a wrong input. Please check your parameters.",
			"stack-trace": string(debug.Stack()),
		})
}

// CheckIfUserIsAllowed checks if the user is allowed to access the data for the given customer
func CheckIfUserIsAllowed(c *gin.Context, customer string) error {
	if InsecureNoAuth {
		zap.S().Debug("InsecureNoAuth is set to true. Skipping user check.")
		return nil
	}

	user := c.MustGet(gin.AuthUserKey)
	if user != customer {
		c.AbortWithStatus(http.StatusUnauthorized)
		zap.S().Infof("User %s unauthorized to access %s", user, internal.SanitizeString(customer))
		return fmt.Errorf("user %s unauthorized to access %s", user, internal.SanitizeString(customer))
	}
	return nil
}
