package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type deleteShiftByAssetIdAndBeginTimestampQueue struct {
	DBAssetID        uint32
	BeginTimeStampMs uint32 `json:"begin_time_stamp"`
}

type deleteShiftByAssetIdAndBeginTimestamp struct {
	BeginTimeStampMs uint32 `json:"begin_time_stamp"`
}

type DeleteShiftByAssetIdAndBeginTimestampHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) Setup() (err error) {
	const queuePathDB = "/data/DeleteShiftByAssetIdAndBeginTimestamp"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload deleteShiftByAssetIdAndBeginTimestamp

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := deleteShiftByAssetIdAndBeginTimestampQueue{
		DBAssetID:        DBassetID,
		BeginTimeStampMs: parsedPayload.BeginTimeStampMs,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
