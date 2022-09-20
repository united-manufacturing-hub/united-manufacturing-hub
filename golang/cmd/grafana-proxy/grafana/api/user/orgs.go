package user

// This file was generated from JSON Schema using quicktype, do not modify it directly.
// To parse and unparse this JSON data, add this code to your project and do:
//
//    user, err := UnmarshalUser(bytes)
//    bytes, err = user.Marshal()

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"io/ioutil"
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

	req, err := http.NewRequest("GET", url, http.NoBody)
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
		bodyBytes, err = ioutil.ReadAll(resp.Body)
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
