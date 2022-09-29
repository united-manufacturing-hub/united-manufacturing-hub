package main

import (
	"github.com/beeker1121/goque"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

const queuePath = "/data/factoryinput/queue"

var queue *goque.Queue

func setupQueue() (err error) {
	queue, err = goque.OpenQueue(queuePath)
	if err != nil {
		zap.S().Errorf("Error opening queue: %v", err)
		return
	}
	return
}

func closeQueue() (err error) {
	err = queue.Close()
	if err != nil {
		zap.S().Errorf("Error closing queue: %v", err)
		return
	}
	return
}

func enqueueMQTT(mqttData MQTTData) (err error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	bytes, err := json.Marshal(mqttData)
	if err != nil {
		return
	}
	_, err = queue.Enqueue(bytes)
	if err != nil {
		return err
	}
	return
}

func dequeueMQTT() (mqttData MQTTData, err error) {
	var item *goque.Item
	// Dequeue is internally atomic
	item, err = queue.Dequeue()

	if err != nil {
		return
	}
	if item != nil {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(item.Value, &mqttData)
	}
	return
}
