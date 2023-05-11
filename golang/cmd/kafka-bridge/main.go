package main

import (
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-bridge/message"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-bridge/processor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"regexp"
	"time"
)

type SendDir string

const (
	ToRemote SendDir = "to_remote"
	ToLocal  SendDir = "to_local"
	Both     SendDir = "both"
)

type TopicMaps []TopicMap

type TopicMap struct {
	Name      string  `json:"name"`
	Topic     string  `json:"topic"`
	Direction SendDir `json:"direction,omitempty"`
}

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION")
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()

	zap.S().Debug("Setting up metrics")
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe(":2112", nil)
		if err != nil {
			zap.S().Errorf("Error starting metrics: %s", err)
		}
	}()

	zap.S().Debugf("Setting up healthcheck")
	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()

	var topicMaps TopicMaps
	err := env.GetAsType("KAFKA_TOPIC_MAP", &topicMaps, true, TopicMaps{})
	if err != nil {
		zap.S().Fatal(err)
	}
	topicMaps.validateTopicMap()

	localKafkaBroker, err := env.GetAsString("LOCAL_KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	remoteKafkaBroker, err := env.GetAsString("REMOTE_KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	groupId, err := env.GetAsString("GROUP_ID", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	useSsl, err := env.GetAsBool("USE_SSL", false, false)
	if err != nil {
		zap.S().Warn(err)
	}

	localClientOptions := kafka.NewClientOptions{
		ConsumerName:      groupId,
		Brokers:           []string{localKafkaBroker},
		Partitions:        6,
		ReplicationFactor: 1,
		EnableTLS:         useSsl,
		ClientID:          "kafka-bridge-local",
	}
	remoteClientOptions := kafka.NewClientOptions{
		ConsumerName:      groupId,
		Brokers:           []string{remoteKafkaBroker},
		Partitions:        6,
		ReplicationFactor: 1,
		EnableTLS:         useSsl,
		ClientID:          "kafka-bridge-remote",
	}

	for _, topicMap := range topicMaps {
		listenTopicRegex, err := regexp.Compile(topicMap.Topic)
		if err != nil {
			zap.S().Fatalf("Error compiling regex: %v", err)
		}

		if topicMap.Direction == ToRemote || topicMap.Direction == Both {
			var toRemoteChan = make(chan kafka.Message, 100)
			var putbackChan = make(chan processor.PutBackChanMsg, 100)
			var shutdownChan = make(chan bool, 1)

			message.Init()
			processor.CreateClient(listenTopicRegex, localClientOptions, remoteClientOptions, toRemoteChan)
			go processor.Start(toRemoteChan)
			go processor.StartPutbackProcessor(topicMap.Name, putbackChan, toRemoteChan)

			go checkDisconnect(shutdownChan)
			reportStats(shutdownChan, toRemoteChan, topicMap.Name)

			zap.S().Info("Shutting down")
			processor.Shutdown()
		}
		if topicMap.Direction == ToLocal || topicMap.Direction == Both {
			var toLocalChan = make(chan kafka.Message, 100)
			var putbackChan = make(chan processor.PutBackChanMsg, 100)
			var shutdownChan = make(chan bool, 1)

			message.Init()
			processor.CreateClient(listenTopicRegex, remoteClientOptions, localClientOptions, toLocalChan)
			go processor.Start(toLocalChan)
			go processor.StartPutbackProcessor(topicMap.Name, putbackChan, toLocalChan)

			go checkDisconnect(shutdownChan)
			reportStats(shutdownChan, toLocalChan, topicMap.Name)

			zap.S().Info("Shutting down")
			processor.Shutdown()
		}
	}
}

func checkDisconnect(shutdownChan chan bool) {
	initialSent, initialRecived := processor.GetStats()
	for {
		time.Sleep(3 * time.Minute)
		sent, recived := processor.GetStats()
		if sent == initialSent && recived == initialRecived {
			zap.S().Info("No messages sent or recived in 3 minutes, shutting down")
			shutdownChan <- true
			return
		}
		initialSent = sent
		initialRecived = recived
	}
}

func reportStats(shutdownChan chan bool, kafkaChan chan kafka.Message, name string) {
	initialSent, initialRecived := processor.GetStats()
	for {
		select {
		case <-shutdownChan:
			zap.S().Info("Shutting down")
			return
		case <-time.After(10 * time.Second):
			sent, recived := processor.GetStats()

			sentPerSecond := (sent - initialSent) / 10
			recivedPerSecond := (recived - initialRecived) / 10
			cacheUsedRaw, cacheMaxRaw, cacheUsed, cacheMax := message.GetCacheSize()
			cachePercentRaw := float64(cacheUsedRaw) / float64(cacheMaxRaw) * 100
			cachePercent := float64(cacheUsed) / float64(cacheMax) * 100
			zap.S().Infof(
				"Processor: %s | Sent: %d (%d/s) Recived: %d (%d/s) | Cached: %d/%d (%.2f%%) | Cached raw: %d/%d (%.2f%%) | Kafka queue: %d",
				name, sent, sentPerSecond, recived, recivedPerSecond, cacheUsed, cacheMax, cachePercent, cacheUsedRaw, cacheMaxRaw, cachePercentRaw, len(kafkaChan))
			initialSent = sent
			initialRecived = recived
		}
	}
}

func (topicMaps *TopicMaps) validateTopicMap() {
	for _, topicMap := range *topicMaps {
		if topicMap.Direction != ToRemote && topicMap.Direction != ToLocal && topicMap.Direction != Both {
			zap.S().Fatalf("Invalid direction %s for topic %s", topicMap.Direction, topicMap.Name)
		}
	}
}
