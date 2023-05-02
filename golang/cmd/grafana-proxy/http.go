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
	"bytes"
	"context"
	"fmt"
	"github.com/cristalhq/base64"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/grafana-proxy/grafana/api/user"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var FactoryInputAPIKey string
var FactoryInputUser string
var FactoryInputBaseURL string
var FactoryInsightBaseUrl string

func SetupRestAPI(factoryInputApiKey, factoryInputUser, factoryInputBaseURL, facoryInsightBaseUrl string) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	FactoryInputAPIKey = factoryInputApiKey
	FactoryInputUser = factoryInputUser
	FactoryInputBaseURL = factoryInputBaseURL
	FactoryInsightBaseUrl = facoryInsightBaseUrl

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
			c.String(http.StatusOK, "online")
		})

	const serviceRoute = "/:service/*data"
	// Version of the API
	v1 := router.Group("/api/v1")
	{
		v1.GET(serviceRoute, getProxyHandler)
		v1.POST(serviceRoute, postProxyHandler)
		v1.OPTIONS(serviceRoute, optionsCORSHAndler)
	}

	err := router.Run(":80")
	if err != nil {
		zap.S().Fatalf("Failed to start rest api: %s", err)
	}
}

func optionsCORSHAndler(c *gin.Context) {

	zap.S().Debugf("optionsCORSHAndler")
	AddCorsHeaders(c)
	c.Status(http.StatusOK)
}

func handleInvalidInputError(c *gin.Context, err error) {

	zap.S().Errorw(
		"Invalid input error",
		"error", err,
	)

	c.String(400, "You have provided a wrong input. Please check your parameters ")
}

type getProxyRequestPath struct {
	Service     string `uri:"service" binding:"required"`
	OriginalURI string `uri:"data" binding:"required"`
}

func handleProxyRequest(c *gin.Context, method string) {

	zap.S().Debugf("getProxyHandler")

	var gPPR getProxyRequestPath
	var err error

	// Failed to parse request into service name and original url
	err = c.BindUri(&gPPR)
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}
	var bodyBytes []byte
	if c.Request.Body != nil {
		bodyBytes, err = io.ReadAll(c.Request.Body)
		if err != nil {
			handleInvalidInputError(c, err)
			return
		}
	}
	zap.S().Warnf("Read %d bytes from original request", len(bodyBytes))

	match, err := regexp.Match("[a-zA-z0-9_\\-?=/]+", []byte(gPPR.OriginalURI))
	if err != nil {
		handleInvalidInputError(c, err)
		return
	}
	// Invalid url
	if !match {
		handleInvalidInputError(c, err)
		return
	}

	zap.S().Debugf("", c.Request)

	// Switch to handle our services
	switch gPPR.Service {
	case "factoryinput":
		HandleFactoryInput(c, gPPR, method, bodyBytes)
	case "factoryinsight":
		HandleFactoryInsight(c, gPPR, method, bodyBytes)
	default:
		{
			zap.S().Warnf("getProxyRequestPath.Service: %s", internal.SanitizeString(gPPR.Service))
			c.AbortWithStatus(http.StatusBadRequest)
		}
	}
}

func AddCorsHeaders(c *gin.Context) {

	origin := c.GetHeader("Origin")
	zap.S().Debugf("Requesting origin: %s", internal.SanitizeString(origin))
	if len(origin) == 0 {
		zap.S().Debugf("Add cors wildcard")
		origin = "*"
	} else {
		zap.S().Debugf("Set cors origin to: %s", internal.SanitizeString(origin))
	}
	c.Header("Access-Control-Allow-Headers", "content-type, Authorization")
	c.Header("Access-Control-Allow-Credentials", "true")
	c.Header("Access-Control-Allow-Origin", internal.SanitizeString(origin))
	c.Header("Access-Control-Allow-Methods", "*")
}

func postProxyHandler(c *gin.Context) {

	// Add cors headers for reply to original requester
	AddCorsHeaders(c)
	handleProxyRequest(c, "POST")
}

func getProxyHandler(c *gin.Context) {

	// Add cors headers for reply to original requester
	AddCorsHeaders(c)
	handleProxyRequest(c, "GET")
}

func IsBase64(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

func HandleFactoryInsight(c *gin.Context, request getProxyRequestPath, method string, bytes []byte) {

	zap.S().Debugf("HandleFactoryInsight")

	authHeader := c.GetHeader("authorization")
	s := strings.Split(authHeader, " ")
	// Basic BASE64Encoded
	if len(s) != 2 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if IsBase64(s[1]) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// HTTP Basic auth not present in request
	if authHeader == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	proxyUrl := strings.TrimPrefix(string(request.OriginalURI), "/")

	var path = "?"
	for k, v := range c.Request.URL.Query() {
		path += fmt.Sprintf("%s=%s&", k, v[0])
	}

	if len(path) == 1 {
		path = ""
	}

	zap.S().Debugf("FactoryInsightBaseUrl: ", FactoryInsightBaseUrl)
	zap.S().Debugf("proxyUrl: ", internal.SanitizeString(proxyUrl))
	zap.S().Debugf("path: ", internal.SanitizeString(path))

	// Validate proxy url
	u, err := url.Parse(fmt.Sprintf("%s%s%s", FactoryInsightBaseUrl, proxyUrl, path))

	if err != nil {
		handleInvalidInputError(c, err)
		return
	}

	DoProxiedRequest(c, u, "", authHeader, method, bytes)
}

// HandleFactoryInput handles proxy requests to factoryinput
func HandleFactoryInput(c *gin.Context, request getProxyRequestPath, method string, bodyBytes []byte) {

	zap.S().Warnf("HandleFactoryInput")

	// Grafana sessionCookie not present in request
	sessionCookie, err := c.Cookie("grafana_session")
	if err != nil {
		zap.S().Warnf("No grafana_session in cookie !")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	zap.S().Warnf("CheckUserLoggedIn")
	// Check if user is logged in
	loggedIn, err := CheckUserLoggedIn(sessionCookie)
	if err != nil {
		zap.S().Warnf("Login error")
		handleInvalidInputError(c, err)
	}

	// Abort if not logged in
	if !loggedIn {
		zap.S().Warnf("StatusForbidden")
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	proxyUrl := strings.TrimPrefix(string(request.OriginalURI), "/")

	zap.S().Warnf("ValidateProxyUrl")
	// Validate proxy url
	u, err := url.Parse(fmt.Sprintf("%sapi/v1/%s", FactoryInputBaseURL, proxyUrl))
	zap.S().Warnf("Proxified URL: %s", internal.SanitizeString(u.String()))
	if err != nil {
		zap.S().Warnf(
			"url.Parse failed: %s",
			internal.SanitizeString(fmt.Sprintf("%sapi/v1/%s", FactoryInputBaseURL, proxyUrl)))
		handleInvalidInputError(c, err)
		return
	}

	// Split proxy url into customer, location, asset, value
	s := strings.Split(u.Path, "/")
	if len(s) != 7 {
		zap.S().Warnf("String split failed: %d", len(s))
		handleInvalidInputError(c, fmt.Errorf("factoryinput url invalid: %d", len(s)))
		return
	}

	zap.S().Debugf("splits: ", internal.SanitizeStringArray(s))

	customer := s[3]

	zap.S().Warnf("GetOrgas")
	// Get grafana organizations of user
	orgas, err := user.GetOrgas(sessionCookie)
	if err != nil {
		zap.S().Warnf("GetOrgas failed %s, %s", err, internal.SanitizeString(sessionCookie))
		handleInvalidInputError(c, err)
		return
	}

	zap.S().Warnf("My Orgas: ", orgas)
	zap.S().Warnf("Customer: %s", internal.SanitizeString(customer))
	// Check if user is in orga, with same name as customer
	allowedOrg := false
	for _, orgsElement := range orgas {
		if orgsElement.Name == customer {
			zap.S().Warnf("%s vs %s", internal.SanitizeString(orgsElement.Name), internal.SanitizeString(customer))
			allowedOrg = true
			break
		}
	}

	// Abort if not in allowed org
	if !allowedOrg {
		zap.S().Warnf("!allowedOrg")
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	ak := fmt.Sprintf(
		"Basic %s",
		base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", FactoryInputUser, FactoryInputAPIKey))))

	DoProxiedRequest(c, u, sessionCookie, ak, method, bodyBytes)
}

func DoProxiedRequest(
	c *gin.Context,
	u *url.URL,
	sessionCookie string,
	authorizationKey string,
	method string,
	bodyBytes []byte) {

	// Proxy request to backend
	client := &http.Client{}

	zap.S().Debugf("Request URL: %s", internal.SanitizeString(u.String()))
	// CORS request !
	if u.String() == "" {
		zap.S().Debugf("CORS Answer")
		c.Status(http.StatusOK)

		if _, err := c.Writer.WriteString("online"); err != nil {
			zap.S().Debugf("Failed to reply to CORS request")
			errx := c.AbortWithError(http.StatusInternalServerError, err)
			if errx != nil {
				zap.S().Errorf("Failed to abort with error: %v", errx)
			}
		}
	} else {
		var err error
		var req *http.Request
		// no nil check required, len(nil slice) is 0
		if len(bodyBytes) > 0 {
			zap.S().Warnf("Request with body bytes: %s", internal.SanitizeByteArray(bodyBytes))
			req, err = http.NewRequestWithContext(context.Background(), method, u.String(), bytes.NewBuffer(bodyBytes))
		} else {
			zap.S().Warnf("Request without body bytes")
			req, err = http.NewRequestWithContext(context.Background(), method, u.String(), http.NoBody)

		}
		if err != nil {
			zap.S().Warnf("Request error: %s", err)
			return
		}
		// Add headers for backend
		if len(sessionCookie) > 0 {
			req.Header.Set("Cookie", fmt.Sprintf("grafana_session=%s", internal.SanitizeString(sessionCookie)))
		}
		req.Header.Set("Authorization", authorizationKey)

		var resp *http.Response
		resp, err = client.Do(req)

		if err != nil {
			zap.S().Debugf("Client.Do error: %v", err)
			c.AbortWithStatus(500)
			return
		}

		var bodyBytesRet []byte
		bodyBytesRet, err = io.ReadAll(resp.Body)
		if err != nil {
			zap.S().Errorf("io.ReadAll error: %v", err)
			c.AbortWithStatus(500)
			return
		}
		err = resp.Body.Close()
		if err != nil {
			zap.S().Errorf("Failed to close response body: %v", err)
			c.AbortWithStatus(500)
			return
		}

		zap.S().Debugf("Backend answer:")
		zap.S().Debugf(string(bodyBytesRet))

		c.Status(resp.StatusCode)

		for a, b := range resp.Header {
			c.Header(a, b[0])
		}
		_, err = c.Writer.Write(bodyBytesRet)
		if err != nil {
			errx := c.AbortWithError(http.StatusInternalServerError, err)
			if errx != nil {
				zap.S().Errorf("Failed to write response: %s", err)
			}
		}
	}
}
