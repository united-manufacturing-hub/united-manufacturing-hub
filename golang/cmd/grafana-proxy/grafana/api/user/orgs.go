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

// This file was generated from JSON Schema using quicktype, do not modify it directly.
// To parse and unparse this JSON data, add this code to your project and do:
//
//    user, err := UnmarshalUser(bytes)
//    bytes, err = user.Marshal()

import (
	"context"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"io"
	"net/http"
)

type Orgs []OrgsElement

func UnmarshalOrgs(data []byte) (Orgs, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	var r Orgs
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *Orgs) Marshal() ([]byte, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	return json.Marshal(r)
}

type OrgsElement struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	OrgID int64  `json:"orgId"`
}

func GetOrgas(sessioncookie string) (Orgs, error) {
	url := fmt.Sprintf("%s/api/user/orgs", GrafanaUrl)
	client := &http.Client{}

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, http.NoBody)
	if err != nil {

		return Orgs{}, err
	}

	req.Header.Set("Cookie", fmt.Sprintf("grafana_session=%s", sessioncookie))

	var resp *http.Response
	resp, err = client.Do(req)
	if err != nil {
		return Orgs{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var bodyBytes []byte
		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			zap.S().Fatalf("Failed to read response body: %v", err)
			return nil, err
		}

		err = resp.Body.Close()
		if err != nil {
			zap.S().Errorf("Failed to close response body: %v", err)
			return nil, err
		}

		orgs, err := UnmarshalOrgs(bodyBytes)
		if err != nil {

			return Orgs{}, err
		}
		return orgs, nil
	}
	return Orgs{}, fmt.Errorf("HTTP Status incorrect: %d", resp.StatusCode)
}
