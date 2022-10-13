package database

import "go.uber.org/zap"

func Init() {
	zap.S().Infof("Initialising database")

}
