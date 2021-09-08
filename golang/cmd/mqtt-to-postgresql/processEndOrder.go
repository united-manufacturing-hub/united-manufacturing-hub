package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type endOrderQueue struct {
	DBAssetID   uint32
	TimestampMs uint64
	OrderName   string
}
type endOrder struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	OrderName   string `json:"order_id"`
}
type EndOrderHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r EndOrderHandler) Setup() (err error) {
	const queuePathDB = "/data/EndOrder"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r EndOrderHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r EndOrderHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r EndOrderHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r EndOrderHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload endOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := endOrderQueue{
		TimestampMs: parsedPayload.TimestampMs,
		OrderName:   parsedPayload.OrderName,
		DBAssetID:   DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
