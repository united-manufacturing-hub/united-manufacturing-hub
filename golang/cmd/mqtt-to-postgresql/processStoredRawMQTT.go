package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
	"time"
)

type StoredRawMQTTHandler struct {
	ProcessPriorityQueue *goque.PriorityQueue
	shutdown             bool
	finishedOldMqtt      bool
}

// NewStoredRawMQTTHandler is a special handler, for storing raw mqtt messages, that couldn't get process due to a server shutdown
func NewStoredRawMQTTHandler() (handler *StoredRawMQTTHandler) {
	const queuePathDB = "/data/StoredRawMQTT"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &StoredRawMQTTHandler{
		ProcessPriorityQueue: priorityQueue,
		shutdown:             false,
	}
	return
}

func (r StoredRawMQTTHandler) Setup() {
	go r.process()
	go r.reprocess()
}
func (r StoredRawMQTTHandler) process() {
	item, err := r.ProcessPriorityQueue.Dequeue()
	if err != nil && err != goque.ErrEmpty {
		zap.S().Warnf("Error dequing in StoredRawMQTTHandler", err)
		r.finishedOldMqtt = true
		return
	}

	for err != goque.ErrEmpty {
		var pt mqttMessage
		errx := item.ToObjectFromJSON(&pt)
		if errx != nil {
			zap.S().Errorf("Stored MQTT message is corrupt !", err, item)
			continue
		}
		err := processMessage(pt.CustomerID, pt.Location, pt.AssetID, pt.Prefix, pt.Payload)
		if err != nil {
			// Postgres doesn't seem to be working, just try again later
			_, err := r.ProcessPriorityQueue.Enqueue(item.Priority, item.Value)
			if err != nil {
				// Failed to re-enqueue
				ShutdownApplicationGraceful()
			}
		}
		item, err = r.ProcessPriorityQueue.Dequeue()
	}

	zap.S().Infof("Finished handling old MQTT messages !")
	r.finishedOldMqtt = true
}

func (r StoredRawMQTTHandler) reprocess() {
	for {
		if r.finishedOldMqtt == false {
			// Old messages are still in the queue, waiting for them to be processed
			time.Sleep(10 * time.Second)
			continue
		}

		item, err := r.ProcessPriorityQueue.Dequeue()
		if err != nil {
			// Sleep if there is any error and just try again later
			time.Sleep(1 * time.Second)
			continue
		}

		for err != goque.ErrEmpty {
			var pt mqttMessage
			errx := item.ToObjectFromJSON(&pt)
			if errx != nil {
				zap.S().Errorf("Stored MQTT message is corrupt !", err, item)
				continue
			}
			err := processMessage(pt.CustomerID, pt.Location, pt.AssetID, pt.Prefix, pt.Payload)
			if err != nil {
				// Postgres doesn't seem to be working, just try again later
				_, err := r.ProcessPriorityQueue.Enqueue(item.Priority, item.Value)
				if err != nil {
					// Failed to re-enqueue
					ShutdownApplicationGraceful()
				}
				time.Sleep(1 * time.Second)
			}
			item, err = r.ProcessPriorityQueue.Dequeue()
		}

	}
}

func (r StoredRawMQTTHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.ProcessPriorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Errorf("Failed to enqueue item, loss of data !", bytes, err)
		return
	}
}

func (r StoredRawMQTTHandler) Shutdown() (err error) {
	zap.S().Warnf("[StoredRawMQTTHandler] shutting down, Queue length: %d", r.ProcessPriorityQueue.Length())
	r.shutdown = true

	err = CloseQueue(r.ProcessPriorityQueue)
	return
}

type mqttMessage struct {
	CustomerID string
	Location   string
	AssetID    string
	Payload    []byte
	Prefix     string
}

func (r StoredRawMQTTHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte, prefix string, recursionDepth int64) {
	zap.S().Debugf("[StoredRawMQTTHandler]")
	var marshal []byte

	newObject := mqttMessage{
		CustomerID: customerID,
		Location:   location,
		AssetID:    assetID,
		Payload:    payload,
		Prefix:     prefix,
	}
	if !ValidateStruct(newObject) {
		zap.S().Errorf("Failed to validate struct of type mqttMessage", newObject)
		return
	}
	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
