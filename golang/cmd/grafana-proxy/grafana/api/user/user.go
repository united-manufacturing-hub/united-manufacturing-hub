package user

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"io"
	"io/ioutil"
	"log"
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

	req, err := http.NewRequest("GET", url, http.NoBody)
	if err != nil {

		return User{}, err
	}

	req.Header.Set("Cookie", fmt.Sprintf("grafana_session=%s", sessioncookie))

	resp, err := client.Do(req)
	if err != nil {

		return User{}, err
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

		user, err := UnmarshalUser(bodyBytes)
		if err != nil {

			return User{}, err
		}
		return user, nil
	}
	return User{}, fmt.Errorf("HTTP Status incorrect: %d", resp.StatusCode)
}
