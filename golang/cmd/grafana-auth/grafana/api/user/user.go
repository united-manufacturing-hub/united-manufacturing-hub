package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
)

func UnmarshalUser(data []byte) (User, error) {
	var r User
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *User) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type User struct {
	ID             int64         `json:"id"`
	Email          string        `json:"email"`
	Name           string        `json:"name"`
	Login          string        `json:"login"`
	Theme          string        `json:"theme"`
	OrgID          int64         `json:"orgId"`
	IsGrafanaAdmin bool          `json:"isGrafanaAdmin"`
	IsDisabled     bool          `json:"isDisabled"`
	IsExternal     bool          `json:"isExternal"`
	AuthLabels     []interface{} `json:"authLabels"`
	UpdatedAt      string        `json:"updatedAt"`
	CreatedAt      string        `json:"createdAt"`
	AvatarURL      string        `json:"avatarUrl"`
}

const GrafanaUrl = "http://localhost:3000"

func GetUser(sessioncookie string) (User, error) {
	url := fmt.Sprintf("%s/api/user", GrafanaUrl)
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("NewRequest failed")
		return User{}, err
	}

	req.Header.Set("Cookie", fmt.Sprintf("grafana_session=%s", sessioncookie))

	fmt.Println("%s", req)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Do failed: %s", err)
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
			fmt.Println("ReadAll failed")
			log.Fatal(err)
		}

		user, err := UnmarshalUser(bodyBytes)
		if err != nil {
			fmt.Println("UnmarshalUser failed")
			return User{}, err
		}
		return user, nil
	}
	return User{}, errors.New(fmt.Sprintf("HTTP Status incorrect: %s", resp.StatusCode))
}
