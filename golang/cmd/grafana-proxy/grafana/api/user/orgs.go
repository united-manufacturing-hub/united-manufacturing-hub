package user

// This file was generated from JSON Schema using quicktype, do not modify it directly.
// To parse and unparse this JSON data, add this code to your project and do:
//
//    user, err := UnmarshalUser(bytes)
//    bytes, err = user.Marshal()

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
)

type Orgs []OrgsElement

func UnmarshalOrgs(data []byte) (Orgs, error) {
	var r Orgs
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *Orgs) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type OrgsElement struct {
	OrgID int64  `json:"orgId"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func GetOrgas(sessioncookie string) (Orgs, error) {
	url := fmt.Sprintf("%s/api/user/orgs", GrafanaUrl)
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {

		return Orgs{}, err
	}

	req.Header.Set("Cookie", fmt.Sprintf("grafana_session=%s", sessioncookie))

	resp, err := client.Do(req)
	if err != nil {

		return Orgs{}, err
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			panic(err)
		}
	}(resp.Body)

	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {

			log.Fatal(err)
		}

		orgs, err := UnmarshalOrgs(bodyBytes)
		if err != nil {

			return Orgs{}, err
		}
		return orgs, nil
	}
	return Orgs{}, fmt.Errorf("hTTP Status incorrect: %s", resp.StatusCode)
}
