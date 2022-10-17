package helpers

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"runtime/debug"
)

func HandleInternalServerError(c *gin.Context, err error) {
	if c == nil {
		panic("HandleInternalServerError: c is nil")
	}
	if err == nil {
		err = errors.New("unknown error")
	}

	erx := internal.SanitizeString(err.Error())
	zap.S().Errorw(
		"Internal server error",
		"error", erx,
	)

	c.JSON(
		http.StatusInternalServerError,
		gin.H{
			"error":       erx,
			"status":      http.StatusInternalServerError,
			"message":     "The server had an internal error.",
			"stack-trace": string(debug.Stack()),
		})
}

func HandleTypeNotFound(c *gin.Context, t any) {
	if c == nil {
		panic("HandleTypeNotFound: c is nil")
	}

	zap.S().Errorw(
		"Type not found",
		t,
	)
	// Get request url from c
	route := c.FullPath()

	c.JSON(
		http.StatusNotFound,
		gin.H{
			"error":       fmt.Sprintf("Type %s not found", t),
			"status":      http.StatusNotFound,
			"message":     fmt.Sprintf("The requested type %s was not found.", t),
			"stack-trace": string(debug.Stack()),
			"route":       route,
		})
}

func HandleInvalidInputError(c *gin.Context, err error) {
	if c == nil {
		panic("HandleInternalServerError: c is nil")
	}
	if err == nil {
		err = errors.New("unknown error")
	}
	erx := internal.SanitizeString(err.Error())
	zap.S().Errorw(
		"Invalid input error",
		"error", erx,
	)

	c.JSON(
		http.StatusBadRequest,
		gin.H{
			"error":       erx,
			"status":      http.StatusBadRequest,
			"message":     "You have provided a wrong input. Please check your parameters.",
			"stack-trace": string(debug.Stack()),
		})
}

// CheckIfUserIsAllowed checks if the user is allowed to access the data for the given customer
func CheckIfUserIsAllowed(c *gin.Context, customer string) error {

	user := c.MustGet(gin.AuthUserKey)
	if user != customer {
		c.AbortWithStatus(http.StatusUnauthorized)
		zap.S().Infof("User %s unauthorized to access %s", user, internal.SanitizeString(customer))
		return fmt.Errorf("user %s unauthorized to access %s", user, internal.SanitizeString(customer))
	}
	return nil
}
