package main

import (
	"errors"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var buildtime string

func main() {
	// zap logging
	log := logger.New("DEVELOPMENT")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	zap.S().Infof("This is kafka-topic-updater build date: %s", buildtime)

	zap.S().Debugf("Setting up healthcheck")

	// pod liveness check
	health := healthcheck.NewHandler()
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(1000000))
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()
	zap.S().Debugf("Starting queue processor")

	// initialize channels for incoming messages
	processorChannel := make(chan *kafka.Message, 100)

	//start up a kafka thing
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOTSTRAP_SERVER")
	internal.SetupKafka(
		kafka.ConfigMap{
			"bootstrap.servers": KafkaBoostrapServer,
			"security_protocol": "plaintext",
			"group.id":          "kafka-topic-updater",
		})
	err := internal.KafkaConsumer.Subscribe("ia.+", nil)
	if err != nil {
		zap.S().Fatalf("failed to subscribe to old topics: %s", err)
		return
	}

	go consume(processorChannel)

	go processing(processorChannel)

	for {
		<-internal.KafkaProducer.Events()
	}
}

func consume(processorChannel chan *kafka.Message) {
	for {
		message, err := internal.KafkaConsumer.ReadMessage(5)
		if err != nil {
			// This is fine, and expected behaviour
			var kafkaError kafka.Error
			ok := errors.As(err, &kafkaError)
			if ok && kafkaError.Code() == kafka.ErrTimedOut {
				// Sleep to reduce CPU usage
				time.Sleep(internal.OneSecond)
				continue
			} else if ok && kafkaError.Code() == kafka.ErrUnknownTopicOrPart {
				time.Sleep(5 * time.Second)
				continue
			} else {
				zap.S().Warnf("Failed to read kafka message: %s", err)
				time.Sleep(5 * time.Second)
				continue
			}

		}
		processorChannel <- message
	}
}

func processing(processorChannel chan *kafka.Message) {
	// declaring regexps for different message types
	re := regexp.MustCompile(internal.KafkaUMHTopicRegex)
	reraw := regexp.MustCompile(`^ia.raw.(-\w_.)+`)
	topicinfo := internal.TopicInformation{}
	for {
		message := <-processorChannel
		topicinfo = *internal.GetTopicInformationCached(*message.TopicPartition.Topic)
		var oldtopicmatch = re.MatchString(*message.TopicPartition.Topic)
		if oldtopicmatch {
			switch {
			case reraw.MatchString(topicinfo.Topic):
				*message.TopicPartition.Topic = strings.Replace(*message.TopicPartition.Topic, "ia.raw", "umh.v1.defaultEnterprise.defaultSite.defaultArea.defaultProductionLine.defaultWorkCell.raw.raw", 1)
			case topicinfo.Topic == "count":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.product.add"
			case topicinfo.Topic == "addOrder":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.job.add"
			case topicinfo.Topic == "startOrder":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.job.start"
			case topicinfo.Topic == "endOrder":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.job.end"
			case topicinfo.Topic == "addShift":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.shift.add"
			case topicinfo.Topic == "deleteShift":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.shift.delete"
			case topicinfo.Topic == "addProduct":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.product-type.add"
			case topicinfo.Topic == "modifyProducedPieces":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.product.overwrite"
			case topicinfo.Topic == "state":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.state.add"
			case topicinfo.Topic == "modifyState":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.state.overwrite"
			case topicinfo.Topic == "activity":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.state.activity"
			case topicinfo.Topic == "detectedAnomaly":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".standard.job.add"
			case topicinfo.Topic == "processValue":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".custom." + strings.Join(topicinfo.ExtendedTopics, ".")
			case topicinfo.Topic == "processValueString":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + ".defaultarea.defaultproductionLine." + topicinfo.AssetId + ".custom." + strings.Join(topicinfo.ExtendedTopics, ".")
			}
			err := internal.KafkaProducer.Produce(message, nil)
			if err != nil {
				zap.S().Warnf("Failed to produce new topic structure %s, %s", err, *message.TopicPartition.Topic)
			}
		}
	}
}
