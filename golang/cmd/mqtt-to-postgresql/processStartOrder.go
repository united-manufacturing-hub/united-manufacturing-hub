package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type startOrderQueue struct {
	DBAssetID   uint32
	TimestampMs uint64
	OrderName   string
}
type startOrder struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	OrderName   string `json:"order_id"`
}
type StartOrderHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r StartOrderHandler) Setup() (err error) {
	const queuePathDB = "/data/StartOrder"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r StartOrderHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r StartOrderHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r StartOrderHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r StartOrderHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload startOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := startOrderQueue{
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
