// Copyright 2025 UMH Systems GmbH
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

package v2

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/hash"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"
	"go.uber.org/zap"
)

// login logs in to the API and returns a JWT token, UUID & the instance name
func login(token string, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) (*LoginResponse, error) {
	var cookieMap = make(map[string]string)
	request, status, err := http.PostRequest[backend_api_structs.InstanceLoginResponse, any](context.Background(), http.LoginEndpoint, nil, map[string]string{
		"Authorization": fmt.Sprint("Bearer ", LoginHash(token)),
	}, &cookieMap, insecureTLS, apiURL, logger)
	if err != nil {
		logger.Warnf("Failed to login (Status %d): %s", status, err)
		return nil, err
	}
	if cookieMap == nil {
		logger.Warnf("No cookie returned")
		return nil, fmt.Errorf("no cookie returned")
	}
	if cookie, ok := cookieMap["token"]; !ok || cookie == "" {
		logger.Warnf("No token cookie returned")
		return nil, fmt.Errorf("no token cookie returned")
	}
	if cookieMap["token"] == "" {
		logger.Warnf("Token cookie is empty")
		return nil, fmt.Errorf("token cookie is empty")
	}

	if request == nil {
		logger.Warnf("No request returned")
		return nil, fmt.Errorf("no request returned")
	}

	// Parse UUID
	uuid, err := uuid.Parse(request.UUID)
	if err != nil {
		logger.Warnf("Failed to parse UUID: %s", err)
		return nil, fmt.Errorf("failed to parse UUID")
	}

	var LoginResponse LoginResponse
	LoginResponse.JWT = cookieMap["token"]
	LoginResponse.UUID = uuid
	LoginResponse.Name = request.Name

	return &LoginResponse, nil
}

func NewLogin(authToken string, insecureTLS bool, apiURL string, logger *zap.SugaredLogger) *LoginResponse {
	var credentials *LoginResponse
	bo := tools.NewBackoff(1*time.Second, 1*time.Second, 60*time.Second, tools.BackoffPolicyLinear)
	var loggedIn bool
	for !loggedIn {
		var err error
		credentials, err = login(authToken, insecureTLS, apiURL, logger)
		if err != nil {
			logger.Warnf("Failed to login: %s", err)
			bo.IncrementAndSleep()
		} else {
			loggedIn = true
		}
	}
	if credentials == nil {
		logger.Error("Login successful but credentials are nil")
		return nil
	}
	logger.Infof("Successfully logged in as %s", credentials.Name)
	return credentials
}

func LoginHash(authToken string) string {
	return hash.Sha3Hash(hash.Sha3Hash(authToken))
}
