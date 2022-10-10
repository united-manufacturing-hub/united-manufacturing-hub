package services

import "github.com/united-manufacturing-hub/united-manufacturing-hub/internal"

func GetStandardMethods() (methods []string, err error) {
	for _, method := range internal.StandardMethods {
		methods = append(methods, method)
	}
	return methods, nil
}
