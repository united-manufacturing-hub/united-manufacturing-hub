package helpers

import "strings"

func StrToBool(integer string) bool {
	integer = strings.ToLower(integer)
	if integer == "1" || integer == "true" || integer == "yes" || integer == "on" || integer == "y" || integer == "t" {
		return true
	} else {
		return false
	}
}
