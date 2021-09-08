package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type modifyStateQueue struct {
	DBAssetID        uint32
	StartTimeStampMs uint32
	EndTimeStampMs   uint32
	NewState         uint32
}

type modifyState struct {
	StartTimeStampMs uint32 `json:"start_time_stamp"`
	EndTimeStampMs   uint32 `json:"end_time_stamp"`
	NewState         uint32 `json:"new_state"`
}

type ModifyStateHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r ModifyStateHandler) Setup() (err error) {
	const queuePathDB = "/data/ModifyState"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r ModifyStateHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r ModifyStateHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r ModifyStateHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r ModifyStateHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload modifyState

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := modifyStateQueue{
		DBAssetID:        DBassetID,
		StartTimeStampMs: parsedPayload.StartTimeStampMs,
		EndTimeStampMs:   parsedPayload.EndTimeStampMs,
		NewState:         parsedPayload.NewState,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
