package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/gvalkov/golang-evdev"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.elastic.co/ecszap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
)

var serialNumber string

var kafkaSendTopic string

var scanOnly bool
var buildtime string

func main() {
	// pprof
	http.ListenAndServe("localhost:1337", nil)
	var logLevel = os.Getenv("LOGGING_LEVEL")
	encoderConfig := ecszap.NewDefaultEncoderConfig()
	var core zapcore.Core
	switch logLevel {
	case "DEVELOPMENT":
		core = ecszap.NewCore(encoderConfig, os.Stdout, zap.DebugLevel)
	default:
		core = ecszap.NewCore(encoderConfig, os.Stdout, zap.InfoLevel)
	}
	logger := zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Infof("This is barcodereader build date: %s", buildtime)

	foundDevice, inputDevice := GetBarcodeReaderDevice()
	if !foundDevice || inputDevice == nil {
		zap.S().Warnf("No barcode reader device found")
		// Restart if no device is found
		os.Exit(1)
	}
	zap.S().Infof("Using device: %v -> %v", foundDevice, inputDevice)

	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	customerID := os.Getenv("CUSTOMER_ID")
	location := os.Getenv("LOCATION")
	assetID := os.Getenv("ASSET_ID")
	serialNumber = os.Getenv("SERIAL_NUMBER")
	scanOnly = os.Getenv("SCAN_ONLY") == "true"

	if !scanOnly {
		kafkaSendTopic = fmt.Sprintf("ia.%s.%s.%s.barcode", customerID, location, assetID)

		internal.SetupKafka(kafka.ConfigMap{
			"bootstrap.servers": KafkaBoostrapServer,
			"security.protocol": "plaintext",
			"group.id":          "barcodereader",
		})
		err := internal.CreateTopicIfNotExists(kafkaSendTopic)
		if err != nil {
			panic(err)
		}

		go internal.StartEventHandler("barcodereader", internal.KafkaProducer.Events(), nil)
	} else {
		zap.S().Infof("Scan only mode")
	}

	go ScanForever(inputDevice, OnScan, OnScanError)

	select {}
}

// GetBarcodeReaderDevice returns the first barcode reader device found, by name or device path.
// If no device is found, it will print all available devices and return false, nil.
func GetBarcodeReaderDevice() (bool, *evdev.InputDevice) {
	// This could be /dev/input/event0, /dev/input/event1, etc.
	devicePath := os.Getenv("INPUT_DEVICE_PATH")
	// This could be "Datalogic ADC, Inc. Handheld Barcode Scanner"
	deviceName := os.Getenv("INPUT_DEVICE_NAME")
	unset := false
	if devicePath == "" && deviceName == "" {
		zap.S().Warnf("No device path or name specified (INPUT_DEVICE_PATH and INPUT_DEVICE_NAME)")
		unset = true
	}

	if devicePath == "" {
		devicePath = "/dev/input/event*"
	}

	zap.S().Infof("Looking for device at path: %v", devicePath)
	devices, err := evdev.ListInputDevices(devicePath)
	if err != nil {
		zap.S().Errorf("Error listing devices: %v", err)
		return false, nil
	}

	if unset {
		for _, inputDevice := range devices {
			if err != nil {
				zap.S().Errorf("Error getting device stat: %v", err)
				continue
			}
			zap.S().Infof("Found device: %v", inputDevice)
		}
		return false, nil
	}

	for _, inputDevice := range devices {
		var stat os.FileInfo
		stat, err = inputDevice.File.Stat()
		if err != nil {
			zap.S().Errorf("Error getting device stat: %v", err)
			continue
		}
		if inputDevice.Name == deviceName || fmt.Sprintf("/dev/input/%v", stat.Name()) == devicePath {
			zap.S().Infof("Found device: %v", inputDevice)
			return true, inputDevice
		}
	}

	return true, nil
}

// BarcodeMessage is the message sent to Kafka, defined by our documentation.
type BarcodeMessage struct {
	TimestampMs int64  `json:"timestamp_ms"`
	Barcode     string `json:"barcode"`
}

// OnScan is called when a barcode is scanned, and produces a message to be sent to Kafka.
func OnScan(scanned string) {
	zap.S().Infof("Scanned: %s\n", scanned)

	if scanOnly {
		return
	}

	message := BarcodeMessage{
		TimestampMs: time.Now().UnixMilli(),
		Barcode:     scanned,
	}
	bytes, err := jsoniter.Marshal(message)
	if err != nil {
		zap.S().Warnf("Error marshalling message: %v (%v)", err, scanned)
		return
	}

	msg := kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic: &kafkaSendTopic,
		},
		Value:   bytes,
		Headers: []kafka.Header{{Key: "origin", Value: []byte(serialNumber)}},
	}
	err = internal.KafkaProducer.Produce(&msg, nil)
	if err != nil {
		zap.S().Warnf("Error producing message: %v (%v)", err, scanned)
		return
	}
}

// OnScanError is called when an error occurs while scanning, it prints the error to stdout, then exits.
func OnScanError(err error) {
	if err == os.ErrClosed {
		zap.S().Errorf("Error: device has been lost :( . Restarting microservice.")
	} else {
		zap.S().Errorf("Fatal error: %v", err)
	}
	ShutdownGracefully()
}

// ShutdownGracefully closes kafka and then exists
func ShutdownGracefully() {
	if !scanOnly {
		internal.ShuttingDownKafka = true

		time.Sleep(internal.FiveSeconds)
	}
	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
