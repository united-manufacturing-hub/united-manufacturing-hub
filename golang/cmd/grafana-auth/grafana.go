package main

import (
	user "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/grafana-auth/grafana/api/user"
)

func CheckUserLoggedIn(sessioncookie string) (bool, error) {
	u, err := user.GetUser(sessioncookie)

	if err != nil {
		return false, err
	}
	return !u.IsDisabled, nil
}
