package v2

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools"
	"go.uber.org/zap"
)

// login logs in to the API and returns a JWT token, UUID & the instance name
func login(token string, insecureTLS bool) (*LoginResponse, error) {
	//	zap.S().Debugf("Attempting to login with: %s", token)
	var cookieMap map[string]string
	cookieMap = make(map[string]string)
	request, err, status := http.PostRequest[backend_api_structs.InstanceLoginResponse, any](nil, http.LoginEndpoint, nil, map[string]string{
		"Authorization": fmt.Sprint("Bearer ", LoginHash(token)),
	}, &cookieMap, insecureTLS)
	if err != nil {
		zap.S().Warnf("Failed to login (Status %d): %s", status, err)
		return nil, err
	}
	if cookieMap == nil {
		zap.S().Warnf("No cookie returned")
		return nil, fmt.Errorf("no cookie returned")
	}
	if cookie, ok := cookieMap["token"]; !ok || cookie == "" {
		zap.S().Warnf("No token cookie returned")
		return nil, fmt.Errorf("no token cookie returned")
	}
	if cookieMap["token"] == "" {
		zap.S().Warnf("Token cookie is empty")
		return nil, fmt.Errorf("token cookie is empty")
	}

	if request == nil {
		zap.S().Warnf("No request returned")
		return nil, fmt.Errorf("no request returned")
	}

	// Parse UUID
	uuid, err := uuid.Parse(request.UUID)
	if err != nil {
		zap.S().Warnf("Failed to parse UUID: %s", err)
		return nil, fmt.Errorf("failed to parse UUID")
	}

	var LoginResponse LoginResponse
	LoginResponse.JWT = cookieMap["token"]
	LoginResponse.UUID = uuid
	LoginResponse.Name = request.Name

	return &LoginResponse, nil
}

func NewLogin(authToken string, insecureTLS bool) *LoginResponse {
	zap.S().Debug("Initial login attempt")
	var credentials *LoginResponse
	bo := tools.NewBackoff(1*time.Second, 1*time.Second, 60*time.Second, tools.BackoffPolicyLinear)
	var loggedIn bool
	for !loggedIn {
		var err error
		credentials, err = login(authToken, insecureTLS)
		if err != nil {
			zap.S().Warnf("Failed to login: %s", err)
			bo.IncrementAndSleep()
			zap.S().Debug("Retrying login attempt")
		} else {
			loggedIn = true
		}
	}
	zap.S().Debugf("SetLoginResponse successful: %s", credentials)
	return credentials
}
