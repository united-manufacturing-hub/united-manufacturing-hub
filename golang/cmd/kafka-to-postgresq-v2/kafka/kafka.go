package kafka

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresq-v2/kafka/custom"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresq-v2/kafka/standard"
	"go.uber.org/zap"
)

func Init() {
	zap.S().Infof("Initialising Kafka")
	standard.Init()
	custom.Init()
}
