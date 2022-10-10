package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
)

var (
	db                      = database.Db
	Mutex                   = database.Mutex
	GracefulShutdownChannel = database.GracefulShutdownChannel
)
