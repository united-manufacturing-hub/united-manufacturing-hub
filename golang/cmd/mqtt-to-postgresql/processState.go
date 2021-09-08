package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type stateQueue struct {
	DBAssetID   uint32
	State       uint32
	TimestampMs uint64
}
type state struct {
	State       uint32 `json:"state"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

type StateHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r StateHandler) Setup() (err error) {
	const queuePathDB = "/data/State"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r StateHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r StateHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r StateHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r StateHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	var parsedPayload state
	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}
	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := stateQueue{
		TimestampMs: parsedPayload.TimestampMs,
		State:       parsedPayload.State,
		DBAssetID:   DBassetID,
	}
	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}
	r.enqueue(marshal, 0)
	return
}
