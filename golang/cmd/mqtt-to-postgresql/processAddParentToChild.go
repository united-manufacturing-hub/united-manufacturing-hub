package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type addParentToChildQueue struct {
	DBAssetID   uint32
	TimestampMs uint64 `json:"timestamp_ms"`
	ChildAID    string `json:"childAID"`
	ParentAID   string `json:"parentAID"`
}

type addParentToChild struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	ChildAID    string `json:"childAID"`
	ParentAID   string `json:"parentAID"`
}
type AddParentToChildHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r AddParentToChildHandler) Setup() (err error) {
	const queuePathDB = "/data/AddParentToChild"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r AddParentToChildHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r AddParentToChildHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r AddParentToChildHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r AddParentToChildHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload addParentToChild

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := addParentToChildQueue{
		DBAssetID:   DBassetID,
		TimestampMs: parsedPayload.TimestampMs,
		ChildAID:    parsedPayload.ChildAID,
		ParentAID:   parsedPayload.ParentAID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
