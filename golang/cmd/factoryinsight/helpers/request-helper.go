package helpers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
)

func HandleInternalServerError(c *gin.Context, err error) {

	zap.S().Errorw(
		"Internal server error",
		"error", err,
	)

	c.String(http.StatusInternalServerError, "The server had an internal error.")
}

func HandleInvalidInputError(c *gin.Context, err error) {

	zap.S().Errorw(
		"Invalid input error",
		"error", internal.SanitizeString(err.Error()),
	)

	c.String(400, "You have provided a wrong input. Please check your parameters.")
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
