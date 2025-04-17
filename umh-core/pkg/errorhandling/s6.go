package errorhandling

import "errors"

var (
	ErrS6ManagerNotInitialized = errors.New("s6 manager not initialized")
)
