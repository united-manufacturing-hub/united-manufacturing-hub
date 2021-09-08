package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type productTagQueue struct {
	DBAssetID   uint32
	TimestampMs uint64  `json:"timestamp_ms"`
	AID         string  `json:"AID"`
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
}

type productTag struct {
	TimestampMs uint64  `json:"timestamp_ms"`
	AID         string  `json:"AID"`
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
}
type ProductTagHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r ProductTagHandler) Setup() (err error) {
	const queuePathDB = "/data/ProductTag"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r ProductTagHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r ProductTagHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r ProductTagHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r ProductTagHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload productTag

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := productTagQueue{
		DBAssetID:   DBassetID,
		TimestampMs: parsedPayload.TimestampMs,
		AID:         parsedPayload.AID,
		Name:        parsedPayload.Name,
		Value:       parsedPayload.Value,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}
	r.enqueue(marshal, 0)
	return
}
