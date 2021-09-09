package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
	"time"
)

type StoredRawMQTTHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

// NewStoredRawMQTTHandler is a special handler, for storing raw mqtt messages, that couldn't get process due to a server shutdown
func NewStoredRawMQTTHandler() (handler *StoredRawMQTTHandler) {
	const queuePathDB = "/data/StoredRawMQTT"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &StoredRawMQTTHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r StoredRawMQTTHandler) Setup() {
	go r.process()
}
func (r StoredRawMQTTHandler) process() {
	item, err := r.pg.Dequeue()
	if err != nil && err != goque.ErrEmpty {
		zap.S().Warnf("Error dequing in StoredRawMQTTHandler", err)
		return
	}

	for err != goque.ErrEmpty {
		var pt mqttMessage
		errx := item.ToObjectFromJSON(&pt)
		if errx != nil {
			zap.S().Errorf("Stored MQTT message is corrupt !", err, item)
			continue
		}
		processMessage(pt.CustomerID, pt.Location, pt.AssetID, pt.Prefix, pt.Payload)
		item, err = r.pg.Dequeue()
	}

	zap.S().Infof("Finished handling old MQTT messages !")
}

func (r StoredRawMQTTHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Errorf("Failed to enqueue item, loss of data !", bytes, err)
		return
	}
}

func (r StoredRawMQTTHandler) Shutdown() (err error) {
	zap.S().Warnf("[StoredRawMQTTHandler] shutting down !")
	r.shutdown = true
	time.Sleep(5 * time.Second)
	err = CloseQueue(r.pg)
	return
}

type mqttMessage struct {
	CustomerID string
	Location   string
	AssetID    string
	Payload    []byte
	Prefix     string
}

func (r StoredRawMQTTHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte, prefix string) {
	zap.S().Debugf("[StoredRawMQTTHandler]")
	var marshal []byte

	newObject := mqttMessage{
		CustomerID: customerID,
		Location:   location,
		AssetID:    assetID,
		Payload:    payload,
		Prefix:     prefix,
	}
	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
