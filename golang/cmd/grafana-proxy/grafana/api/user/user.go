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

package user

import (
	"context"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"io"
	"net/http"
)

func UnmarshalUser(data []byte) (User, error) {
	var r User
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *User) Marshal() ([]byte, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	return json.Marshal(r)
}

type User struct {
	UpdatedAt      string        `json:"updatedAt"`
	Email          string        `json:"email"`
	Name           string        `json:"name"`
	Login          string        `json:"login"`
	Theme          string        `json:"theme"`
	AvatarURL      string        `json:"avatarUrl"`
	CreatedAt      string        `json:"createdAt"`
	AuthLabels     []interface{} `json:"authLabels"`
	ID             int64         `json:"id"`
	OrgID          int64         `json:"orgId"`
	IsExternal     bool          `json:"isExternal"`
	IsDisabled     bool          `json:"isDisabled"`
	IsGrafanaAdmin bool          `json:"isGrafanaAdmin"`
}

const GrafanaUrl = "http://factorycube-server-grafana:8080"

func GetUser(sessioncookie string) (User, error) {
	url := fmt.Sprintf("%s/api/user", GrafanaUrl)
	client := &http.Client{}

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, http.NoBody)
	if err != nil {

		return User{}, err
	}

	req.Header.Set("Cookie", fmt.Sprintf("grafana_session=%s", sessioncookie))

	resp, err := client.Do(req)
	if err != nil {
		return User{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var bodyBytes []byte
		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			zap.S().Fatalf("Failed to read response body: %v", err)
			return User{}, err
		}
		err = resp.Body.Close()
		if err != nil {
			zap.S().Errorf("Failed to close response body: %v", err)
			return User{}, err
		}

		user, err := UnmarshalUser(bodyBytes)
		if err != nil {

			return User{}, err
		}
		return user, nil
	}
	return User{}, fmt.Errorf("HTTP Status incorrect: %d", resp.StatusCode)
}
