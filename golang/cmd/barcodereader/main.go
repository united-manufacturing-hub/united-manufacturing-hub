// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux
// +build linux

package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	evdev "github.com/gvalkov/golang-evdev"
	"github.com/heptiolabs/healthcheck"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strconv"
	"time"
)

var kafkaSendTopic string

var scanOnly bool
var buildtime string

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
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

	foundDevice, inputDevice := GetBarcodeReaderDevice()
	if !foundDevice || inputDevice == nil {
		zap.S().Warnf("No barcode reader device found")
		// Restart if no device is found
		os.Exit(1)
	}
	zap.S().Infof("Using device: %v -> %v", foundDevice, inputDevice)

	KafkaBoostrapServer, KafkaBootstrapServerEnvSet := os.LookupEnv("KAFKA_BOOTSTRAP_SERVER")
	if !KafkaBootstrapServerEnvSet {
		zap.S().Fatal("Kafka bootstrap server (KAFKA_BOOTSTRAP_SERVER) must be set")
	}
	customerID, customerIDEnvSet := os.LookupEnv("CUSTOMER_ID")
	if !customerIDEnvSet {
		zap.S().Fatal("Customer ID (CUSTOMER_ID) must be set")
	}
	location, locationEnvSet := os.LookupEnv("LOCATION")
	if !locationEnvSet {
		zap.S().Fatal("location (LOCATION) must be set")
	}
	assetID, assetIDEnvSet := os.LookupEnv("ASSET_ID")
	if !assetIDEnvSet {
		zap.S().Fatal("Asset ID (ASSET_ID) must be set")
	}
	var scanOnlyEnvSet bool
	var scanOnlyString string
	scanOnlyString, scanOnlyEnvSet = os.LookupEnv("SCAN_ONLY")
	if !scanOnlyEnvSet {
		zap.S().Fatal("scan only (SCAN_ONLY) must be set")
	}
	var err error
	scanOnly, err = strconv.ParseBool(scanOnlyString)
	if err != nil {
		zap.S().Fatal("scan only (SCAN_ONLY) must be set to true or false")
	}

	if !scanOnly {
		kafkaSendTopic = fmt.Sprintf("ia.%s.%s.%s.barcode", customerID, location, assetID)

		internal.SetupKafka(
			kafka.ConfigMap{
				"bootstrap.servers": KafkaBoostrapServer,
				"security.protocol": "plaintext",
				"group.id":          "barcodereader",
			})
		err := internal.CreateTopicIfNotExists(kafkaSendTopic)
		if err != nil {
			zap.S().Fatalf("Failed to create topic: %v", err)
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
	devicePath, devicePathEnvSet := os.LookupEnv("INPUT_DEVICE_PATH")
	if !devicePathEnvSet {
		zap.S().Fatal("Device Path (DEVICE_PATH) must be set")
	}
	// This could be "Datalogic ADC, Inc. Handheld Barcode Scanner"
	deviceName, deviceNameEnvSet := os.LookupEnv("INPUT_DEVICE_NAME")
	if !deviceNameEnvSet {
		zap.S().Fatal("Device Name (DEVICE_NAME) must be set")
	}
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
		Value: bytes,
	}
	err = internal.Produce(internal.KafkaProducer, &msg, nil)
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
