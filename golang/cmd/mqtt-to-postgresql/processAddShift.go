package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type addShiftQueue struct {
	DBAssetID      uint32
	TimestampMs    uint64
	TimestampMsEnd uint64
}
type addShift struct {
	TimestampMs    uint64 `json:"timestamp_ms"`
	TimestampMsEnd uint64 `json:"timestamp_ms_end"`
}

type AddShiftHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r AddShiftHandler) Setup() (err error) {
	const queuePathDB = "/data/state/AddShift"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r AddShiftHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r AddShiftHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r AddShiftHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r AddShiftHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload addShift

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := addShiftQueue{
		TimestampMs:    parsedPayload.TimestampMs,
		TimestampMsEnd: parsedPayload.TimestampMsEnd,
		DBAssetID:      DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
