package connection

import "errors"

var (
	// ErrServiceNotExist indicates the requested service does not exist
	ErrServiceNotExist = errors.New("connection service does not exist")
	// ErrServiceAlreadyExists indicates the service already exists
	ErrServiceAlreadyExists = errors.New("connection service already exists")
)
