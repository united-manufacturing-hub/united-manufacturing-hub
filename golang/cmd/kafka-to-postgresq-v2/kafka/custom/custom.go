package custom

import "go.uber.org/zap"

func Init() {
	zap.S().Infof("Initialising custom datastream")

}
