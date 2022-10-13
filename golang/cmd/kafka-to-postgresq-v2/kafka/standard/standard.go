package standard

import "go.uber.org/zap"

func Init() {
	zap.S().Infof("Initialising standard datastream")

}
