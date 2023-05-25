package main

import (
	"bufio"
	"fmt"
	"github.com/heptiolabs/healthcheck"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper/pkg/kafka"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"os"
	"regexp"
	"strings"
	"syscall"
	"unsafe"
)

const (
	inputEventSize = 24
	eventFilePath  = "/dev/input/event0"
)

type inputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

// https://en.wikipedia.org/wiki/British_and_American_keyboards#/media/File:KB_United_States-NoAltGr.svg
var keycodeToChar = map[uint16]rune{
	2: '1', 3: '2', 4: '3', 5: '4', 6: '5', 7: '6', 8: '7', 9: '8', 10: '9', 11: '0', 12: '-', 13: '=',
	16: 'q', 17: 'w', 18: 'e', 19: 'r', 20: 't', 21: 'y', 22: 'u', 23: 'i', 24: 'o', 25: 'p', 26: '[', 27: ']',
	30: 'a', 31: 's', 32: 'd', 33: 'f', 34: 'g', 35: 'h', 36: 'j', 37: 'k', 38: 'l', 39: ';', 40: '\'',
	44: 'z', 45: 'x', 46: 'c', 47: 'v', 48: 'b', 49: 'n', 50: 'm', 51: ',', 52: '.', 53: '/',
}

var keycodeToCharShift = map[uint16]rune{
	2: '!', 3: '@', 4: '#', 5: '$', 6: '%', 7: '^', 8: '&', 9: '*', 10: '(', 11: ')', 12: '_', 13: '+',
	16: 'Q', 17: 'W', 18: 'E', 19: 'R', 20: 'T', 21: 'Y', 22: 'U', 23: 'I', 24: 'O', 25: 'P', 26: '{', 27: '}',
	30: 'A', 31: 'S', 32: 'D', 33: 'F', 34: 'G', 35: 'H', 36: 'J', 37: 'K', 38: 'L', 39: ':', 40: '"',
	44: 'Z', 45: 'X', 46: 'C', 47: 'V', 48: 'B', 49: 'N', 50: 'M', 51: '<', 52: '>', 53: '?',
}

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	internal.Initfgtrace()
	// Prometheus
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

	listDevices()
	initKafka()
	scan()
}

var client *kafka.Client
var kafkaSendTopic string

func initKafka() {
	scanOnly, err := env.GetAsBool("SCAN_ONLY", true, false)
	if err != nil {
		zap.S().Fatal(err)
	}
	if scanOnly {
		zap.S().Infof("SCAN_ONLY is set to true, will not connect to Kafka")
		return
	}

	var KafkaBoostrapServer string
	KafkaBoostrapServer, err = env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	var customerID string
	customerID, err = env.GetAsString("CUSTOMER_ID", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	var location string
	location, err = env.GetAsString("LOCATION", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	var assetID string
	assetID, err = env.GetAsString("ASSET_ID", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}

	kafkaSendTopic = fmt.Sprintf("ia.%s.%s.%s.barcode", customerID, location, assetID)
	zap.S().Infof("Sending to Kafka topic %s", kafkaSendTopic)

	client, err = kafka.NewKafkaClient(&kafka.NewClientOptions{
		ListenTopicRegex:  nil,
		ConsumerName:      "barcodereader",
		Brokers:           []string{KafkaBoostrapServer},
		StartOffset:       0,
		Partitions:        6,
		ReplicationFactor: 1,
		EnableTLS:         false,
		SenderTag: kafka.SenderTag{
			Enabled: true,
		},
	})
	if err != nil {
		zap.S().Fatal(err)
	}
}

func sendMessage(payload []byte) {
	if client == nil {
		zap.S().Debugf("Kafka client is nil, not sending message")
		return
	}

	err := client.EnqueueMessage(kafka.Message{
		Topic:  kafkaSendTopic,
		Value:  payload,
		Header: nil,
		Key:    nil,
	})
	if err != nil {
		zap.S().Errorf("Error sending message to Kafka: %s", err)
	}
}

func listDevices() {
	file, err := os.Open("/proc/bus/input/devices")
	if err != nil {
		zap.S().Info("Error opening /proc/bus/input/devices:", err)
		return
	}
	defer file.Close()

	keyboardDevices := make(map[string]string)

	scanner := bufio.NewScanner(file)
	var dev string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "I:") {
			dev = ""
		}
		if strings.HasPrefix(line, "N: Name=") {
			// remove "N: Name=" prefix
			dev = line[8:]
		}
		if strings.HasPrefix(line, "H: Handlers=") {
			handlers := strings.Split(line, "=")[1]
			if strings.Contains(handlers, "kbd") {
				// strip off the leading and trailing spaces and "
				dev = strings.TrimSpace(dev)
				dev = strings.Trim(dev, "\"")

				// get the event number using regex
				re := regexp.MustCompile(`event\d+`)
				event := re.FindString(line)

				keyboardDevices[dev] = event
			}
		}
	}

	if err = scanner.Err(); err != nil {
		zap.S().Fatalf("Error reading /proc/bus/input/devices:", err)
		return
	}

	if len(keyboardDevices) == 0 {
		zap.S().Fatalf("No keyboard devices found")
	}

	zap.S().Info("Keyboard input devices:")
	for device, event := range keyboardDevices {
		zap.S().Infof("%s [/dev/input/%s]\n", device, event)
	}
}

func scan() {
	f, err := os.Open(eventFilePath)
	if err != nil {
		zap.S().Infof("Failed to open input event file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	buffer := make([]byte, inputEventSize)

	var shiftNext bool
	var buf []byte
	for {
		_, err = f.Read(buffer)
		if err != nil {
			zap.S().Infof("Failed to read input event: %v\n", err)
			os.Exit(1)
		}

		event := *(*inputEvent)(unsafe.Pointer(&buffer[0]))

		if event.Type == 1 && event.Value == 1 {
			if event.Code == 28 {
				// print buf
				zap.S().Infof("Decoded: %s\n", buf)
				sendMessage(buf)
				buf = []byte{}
				continue
			}
			if event.Code == 42 || event.Code == 54 {
				shiftNext = true
				continue
			}

			var char rune
			if shiftNext {
				char = keycodeToCharShift[event.Code]
				shiftNext = false
			} else {
				char = keycodeToChar[event.Code]
			}
			zap.S().Debugf("%s [%d]\n", string(char), event.Code)
			if char != 0 {
				buf = append(buf, byte(char))
			} else {
				zap.S().Infof("Unknown key code: %d\n", event.Code)
			}
		}
	}
}
