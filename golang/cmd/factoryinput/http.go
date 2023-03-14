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

package main

import (
	"errors"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"io"
	"net/http"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SetupRestAPI initializes the REST API and starts listening
func SetupRestAPI(accounts gin.Accounts, version string) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Add a ginzap middleware, which:
	//   - Logs all requests, like a combined access and error log.
	//   - Logs to stdout.
	//   - RFC3339 with UTC time format.
	router.Use(ginzap.Ginzap(zap.L(), time.RFC3339, true))

	// Logs all panic to error log
	//   - stack means whether output the stack info.
	router.Use(ginzap.RecoveryWithZap(zap.L(), true))

	// Healthcheck
	router.GET(
		"/", func(c *gin.Context) {
			if shutdownEnabled {
				c.String(http.StatusOK, "shutdown")
			} else {
				c.String(http.StatusOK, "online")
			}
		})

	apiString := fmt.Sprintf("/api/v%s", version)

	// Version of the API
	v1 := router.Group(apiString, gin.BasicAuth(accounts))
	{
		// WARNING: Need to check in each specific handler whether the user is actually allowed to access it, so that valid user "ia" cannot access data for customer "abc"
		v1.POST("/:customer/:location/:asset/:value", postMQTTHandler)
	}

	err := router.Run(":80")
	if err != nil {
		zap.S().Errorf("Failed to bind to port 80", err)
		ShutdownApplicationGraceful()
		return
	}
}

func handleInternalServerError(c *gin.Context, err error) {

	zap.S().Errorw(
		"Internal server error",
		"error", internal.SanitizeString(err.Error()),
	)

	c.String(http.StatusInternalServerError, "The server had an internal error.")
}

func handleInvalidInputError(c *gin.Context, err error) {

	zap.S().Errorw(
		"Invalid input error",
		"error", err,
	)

	c.String(400, "You have provided a wrong input. Please check your parameters")
}

// Access handler
func checkIfUserIsAllowed(c *gin.Context, customer string) error {

	user := c.MustGet(gin.AuthUserKey)
	if user != customer {
		c.AbortWithStatus(http.StatusUnauthorized)
		zap.S().Infof("User %s unauthorized to access %s", user, internal.SanitizeString(customer))
		return fmt.Errorf("user %s unauthorized to access %s", user, internal.SanitizeString(customer))
	}
	return nil
}

func IsJSON(str string) bool {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	var js jsoniter.RawMessage
	return json.Unmarshal([]byte(str), &js) == nil
}

type postMQTTRequest struct {
	Customer string `uri:"customer" binding:"required"`
	Location string `uri:"location" binding:"required"`
	Asset    string `uri:"asset" binding:"required"`
	Value    string `uri:"value" binding:"required"`
}

type MQTTData struct {
	Customer string
	Location string
	Asset    string
	Value    string
	JSONData string
}

func postMQTTHandler(c *gin.Context) {

	jsonBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		handleInvalidInputError(c, err)
	}

	jsonData := string(jsonBytes)
	zap.S().Warnf("jsonData: %s", internal.SanitizeString(jsonData))

	if !IsJSON(jsonData) {
		handleInvalidInputError(c, errors.New("input is not valid JSON"))
	}

	var postMQTTRequestInstance postMQTTRequest
	err = c.BindUri(&postMQTTRequestInstance)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	// Check whether user has access to that customer
	err = checkIfUserIsAllowed(c, postMQTTRequestInstance.Customer)
	if err != nil {
		handleInternalServerError(c, err)
		return
	}

	err = enqueueMQTT(
		MQTTData{
			Customer: postMQTTRequestInstance.Customer,
			Location: postMQTTRequestInstance.Location,
			Asset:    postMQTTRequestInstance.Asset,
			Value:    postMQTTRequestInstance.Value,
			JSONData: jsonData,
		})
	if err != nil {
		handleInternalServerError(c, err)
		return
	}
}
