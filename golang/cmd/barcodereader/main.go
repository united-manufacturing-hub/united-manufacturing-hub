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
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strings"
	"time"
)

var kafkaSendTopic string

var scanOnly bool

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
		// Restart if no device is found
		zap.S().Fatalf("No barcode reader device found")
	}
	zap.S().Infof("Using device: %v -> %v", foundDevice, inputDevice)

	KafkaBoostrapServer, err := env.GetAsString("KAFKA_BOOTSTRAP_SERVER", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	customerID, err := env.GetAsString("CUSTOMER_ID", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	location, err := env.GetAsString("LOCATION", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	assetID, err := env.GetAsString("ASSET_ID", true, "")
	if err != nil {
		zap.S().Fatal(err)
	}
	scanOnly, err := env.GetAsBool("SCAN_ONLY", true, false)

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
	devicePath, _ := env.GetAsString("INPUT_DEVICE_PATH", false, "/dev/input/event*")
	// This could be "Datalogic ADC, Inc. Handheld Barcode Scanner"
	deviceName, _ := env.GetAsString("INPUT_DEVICE_NAME", false, "")

	// Check if devicePath contains a wildcard and get all devices in the path
	if strings.Contains(devicePath, "*") {
		devices, err := evdev.ListInputDevices(devicePath)
		if err != nil {
			zap.S().Errorf("Error listing devices: %v", err)
			return false, nil
		}
		// If deviceName is set, look for the device in the path
		if deviceName != "" {
			for _, inputDevice := range devices {
				if inputDevice.Name == deviceName {
					zap.S().Infof("Found device: %+v", inputDevice)
					return true, inputDevice
				}
			}
			zap.S().Warnf("Device %s not found in path %s", deviceName, devicePath)
		}

		// If deviceName is not set or the device is not found, print all devices in the path and return (false, nil)
		zap.S().Infof("Devices in path %s:", devicePath)
		for _, inputDevice := range devices {
			zap.S().Infof("%+v", inputDevice)
		}
		return false, nil
	}

	// Check if devicePath is a valid device
	inputDevice, err := evdev.Open(devicePath)
	if err != nil {
		zap.S().Errorf("Error opening device: %v", err)
		return false, nil
	}

	// If deviceName is set, check if it matches the device at devicePath
	switch deviceName {
	case "":
		zap.S().Infof("Found device: %v", inputDevice)
		return true, inputDevice
	case inputDevice.Name:
		zap.S().Infof("Found device: %v", inputDevice)
		return true, inputDevice
	default:
		zap.S().Fatalf("Device name (%s) and path (%s) do not refer to the same device", deviceName, devicePath)
		return false, nil
	}
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
