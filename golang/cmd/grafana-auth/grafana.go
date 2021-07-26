package main

import (
	"fmt"
	user "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/grafana-auth/grafana/api/user"
)

func CheckUserLoggedIn(sessioncookie string) (bool, error) {
	u, err := user.GetUser(sessioncookie)
	fmt.Println("Logged in user: ", u)
	if err != nil {
		return false, err
	}
	return !u.IsDisabled, nil
}
