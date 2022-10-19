package main

import (
	"encoding/json"
	_ "errors"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	_ "github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/heptiolabs/healthcheck"
	"github.com/pkg/errors"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	_ "github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"net/http"
	_ "net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var buildtime string
var shutdownEnabled bool

// initialize channels for incoming messages
var processorChannel = make(chan *kafka.Message, 100)
var enterprise = os.Getenv("ENTERPRISE")
var site = os.Getenv("SITE_NAME")
var area = os.Getenv("AREA_NAME")
var prodline = os.Getenv("PRODUCTION_LINE")
var workcell = os.Getenv("WORK_CELL")

func main() {
	// zap logging
	log := logger.New("LOGGING_LEVEL")
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
	health.AddReadinessCheck("shutdownEnabled", isShutdownEnabled())
	go func() {
		/* #nosec G114 */
		err := http.ListenAndServe("0.0.0.0:8086", health)
		if err != nil {
			zap.S().Errorf("Error starting healthcheck: %s", err)
		}
	}()
	zap.S().Debugf("Starting queue processor")

	//start up a kafka thing
	KafkaBoostrapServer := os.Getenv("KAFKA_BOOTSTRAP_SERVER")
	internal.SetupKafka(
		kafka.ConfigMap{
			"bootstrap.servers": KafkaBoostrapServer,
			"security.protocol": "plaintext",
			"group.id":          "kafka-topic-updater",
		})
	err := internal.KafkaConsumer.Subscribe("^ia.+", nil)
	if err != nil {
		zap.S().Fatalf("failed to subscribe to old topics: %s", err)
		return
	}

	go consume(processorChannel)

	go processing(processorChannel)

	go event()

	// Allow graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)

	go func() {
		// before you trapped SIGTERM your process would
		// have exited, so we are now on borrowed time.

		sig := <-sigs

		// Log the received signal
		zap.S().Infof("Received SIGTERM %s", sig)

		// ... close TCP connections here.
		ShutdownApplicationGraceful()

	}()

	select {} // block forever
}

func consume(processorChannel chan *kafka.Message) {
	for !shutdownEnabled {
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
		zap.S().Debugf("consuming message")
		processorChannel <- message

	}
}

// TODO might be more performant to switch it up and have a function for each topic?
func processing(processorChannel chan *kafka.Message) {
	// declaring regexps for different message types
	re := regexp.MustCompile(internal.KafkaUMHTopicRegex)
	reraw := regexp.MustCompile(`^ia\.raw\.(\d|-|\w|_|\.)+`)
	topicinfo := internal.TopicInformation{}
	for {
		message := <-processorChannel
		zap.S().Debugf("processing message: %s", *message.TopicPartition.Topic)
		topicinfo = *internal.GetTopicInformationCached(*message.TopicPartition.Topic)
		var oldtopicmatch = re.MatchString(*message.TopicPartition.Topic)
		if oldtopicmatch {
			zap.S().Debugf("must terminate old topic structure  beep boop")
			switch {
			case reraw.MatchString(*message.TopicPartition.Topic):
				zap.S().Debugf("recognized a raw message")
				newraw := "umh.v1." + enterprise + "." + site + "." + area + "." + prodline + "." + workcell + ".raw"
				*message.TopicPartition.Topic = strings.Replace(*message.TopicPartition.Topic, "ia.raw", newraw, 1)
				zap.S().Debugf("new topic: %s", *message.TopicPartition.Topic)
			case topicinfo.Topic == "count":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.product.add"
				var payload datamodel.Count
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case count: %s", err)
				}
				// producttypeid is not present in the current message structure, but required in the new one, so it declares 1234
				var producttypeid uint64 = 1234
				newpayload := datamodel.Productadd{Timestampend: payload.TimestampMs, Producttypeid: producttypeid, Scrap: payload.Scrap, Totalamount: payload.Count}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "addOrder":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.job.add"
				var payload datamodel.AddOrder
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case addOrder: %s", err)
				}
				newpayload := datamodel.Jobadd{ProductType: payload.ProductId, Jobid: payload.OrderId, Targetamount: payload.TargetUnits}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "startOrder":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.job.start"
				var payload datamodel.StartOrder
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case startOrder: %s", err)
				}
				newpayload := datamodel.Jobstart{Jobid: payload.OrderId, Timestampbegin: payload.TimestampMs}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "endOrder":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.job.end"
				var payload datamodel.EndOrder
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case endOrder: %s", err)
				}
				newpayload := datamodel.Jobend{TimestampEnd: payload.TimestampMs, Jobid: payload.OrderId}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "addShift":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.shift.add"
				var payload datamodel.AddShift
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case addShift: %s", err)
				}
				newpayload := datamodel.Shiftadd{Timestampbegin: payload.TimestampMs, Timestampend: payload.TimestampMsEnd}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "deleteShift":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.shift.delete"
				var payload datamodel.DeleteShift
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case deleteShift: %s", err)
				}
				newpayload := datamodel.Shiftdelete{Timestampbegin: uint64(payload.TimeStampMs)}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "addProduct":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.product-type.add"
				var payload datamodel.AddProduct
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case addProduct: %s", err)
				}
				newpayload := datamodel.Producttypeadd{ProductId: payload.ProductId, Cycletimeinseconds: payload.TimePerUnitInSeconds}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "modifyProducedPieces":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.product.overwrite"
				var payload datamodel.ModifyProducedPieces
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case modifyProducedPieces: %s", err)
				}
				newpayload := datamodel.Productoverwrite{Timestampend: payload.TimestampMs, Totalamount: payload.Count, Scrap: payload.Scrap}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "state":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.state.add"
				var payload datamodel.State
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case state: %s", err)
				}
				newpayload := datamodel.Stateadd{Timestampbegin: payload.TimestampMs, State: payload.State}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "modifyState":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.state.overwrite"
				var payload datamodel.ModifyState
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case modifyState: %s", err)
				}
				newpayload := datamodel.Stateoverwrite{Timestampbegin: payload.StartTimeStampMs, Timestampend: payload.EndTimeStampMs, State: uint64(payload.NewState)}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "activity":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.state.activity"
				var payload datamodel.Activity
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case activity: %s", err)
				}
				newpayload := datamodel.Stateactivity{Timestampbegin: payload.TimestampMs, Activity: payload.Activity}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "detectedAnomaly":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".standard.state.reason"
				var payload datamodel.DetectedAnomaly
				err := json.Unmarshal(message.Value, &payload)
				if err != nil {
					zap.S().Errorf("Error unmarshaling json: case activity: %s", err)
				}
				newpayload := datamodel.Statereason{Timestampbegin: payload.TimestampMs, Reason: payload.DetectedAnomaly}
				message.Value, _ = json.Marshal(newpayload)
			case topicinfo.Topic == "processValue":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".custom." + strings.Join(topicinfo.ExtendedTopics, ".")
			case topicinfo.Topic == "processValueString":
				*message.TopicPartition.Topic = "umh.v1." + topicinfo.CustomerId + "." + topicinfo.Location + "." + area + "." + prodline + "." + topicinfo.AssetId + ".custom." + strings.Join(topicinfo.ExtendedTopics, ".")
			}
		}
		zap.S().Debugf("producing message> %s", *message.TopicPartition.Topic)
		err := internal.KafkaProducer.Produce(message, nil)
		if err != nil {
			zap.S().Warnf("Failed to produce new topic structure %s, %s", err, *message.TopicPartition.Topic)
		}

	}
}

func isShutdownEnabled() healthcheck.Check {
	return func() error {
		if shutdownEnabled {
			return fmt.Errorf("shutdown")
		}
		return nil
	}
}

func ShutdownApplicationGraceful() {
	zap.S().Infof("Shutting down application")
	shutdownEnabled = true

	for len(processorChannel) > 0 {
		time.Sleep(time.Second)
	}

	zap.S().Infof("Successful shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}

func event() {
	for {
		<-internal.KafkaProducer.Events()
	}
}
