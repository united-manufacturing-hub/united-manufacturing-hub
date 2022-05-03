package main

import (
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/gvalkov/golang-evdev"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"os"
	"time"
)

var serialNumber string

var kafkaSendTopic string

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	zap.ReplaceGlobals(logger)

	zap.S().Infof("Starting barcode reader")

	foundDevice, inputDevice := GetBarcodeReaderDevice()
	zap.S().Infof("Using device: %v -> %v", foundDevice, inputDevice)
	if !foundDevice {
		os.Exit(1)
	}

	KafkaBoostrapServer := os.Getenv("KAFKA_BOOSTRAP_SERVER")
	customerID := os.Getenv("CUSTOMER_ID")
	location := os.Getenv("LOCATION")
	assetID := os.Getenv("ASSET_ID")
	serialNumber = os.Getenv("SERIAL_NUMBER")

	kafkaSendTopic = fmt.Sprintf("ia/%s/%s/%s/barcode", customerID, location, assetID)

	internal.SetupKafka(kafka.ConfigMap{
		"bootstrap.servers": KafkaBoostrapServer,
		"security.protocol": "plaintext",
		"group.id":          "barcodereader",
	})
	err = internal.CreateTopicIfNotExists(kafkaSendTopic)
	if err != nil {
		panic(err)
	}

	go internal.StartEventHandler("barcodereader", internal.KafkaProducer.Events(), nil)

	go ScanForever(inputDevice, OnScan, OnScanError)

	select {}
}

func GetBarcodeReaderDevice() (bool, *evdev.InputDevice) {
	// This could be /dev/input/event0, /dev/input/event1, etc.
	devicePath := os.Getenv("INPUT_DEVICE_PATH")
	// This could be "Datalogic ADC, Inc. Handheld Barcode Scanner"
	deviceName := os.Getenv("INPUT_DEVICE_NAME")
	unset := false
	if devicePath == "" && deviceName == "" {
		zap.S().Infof("No device path or name specified (INPUT_DEVICE_PATH and INPUT_DEVICE_NAME)")
		unset = true
	}

	if devicePath == "" {
		devicePath = "/dev/input/event*"
	}

	zap.S().Infof("Looking for device at path: %v", devicePath)
	devices, err := evdev.ListInputDevices(devicePath)
	if err != nil {
		zap.L().Error("Error listing devices", zap.Error(err))
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

type BarcodeMessage struct {
	TimestampMs int64  `json:"timestamp_ms"`
	Barcode     string `json:"barcode"`
}

func OnScan(scanned string) {
	zap.S().Infof("Scanned: %s\n", scanned)

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

func OnScanError(err error) {
	zap.S().Infof("Error: %s\n", err)
	ShutdownGracefully()
}

func ShutdownGracefully() {
	internal.ShuttingDownKafka = true

	time.Sleep(internal.FiveSeconds)

	zap.S().Infof("Successfull shutdown. Exiting.")

	// Gracefully exit.
	// (Use runtime.GoExit() if you need to call defers)
	os.Exit(0)
}
