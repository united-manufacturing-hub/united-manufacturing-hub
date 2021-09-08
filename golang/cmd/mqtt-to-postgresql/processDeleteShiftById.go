package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type deleteShiftByIdQueue struct {
	DBAssetID uint32
	ShiftId   uint32 `json:"shift_id"`
}

type deleteShiftById struct {
	ShiftId uint32 `json:"shift_id"`
}

type DeleteShiftByIdHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r DeleteShiftByIdHandler) Setup() (err error) {
	const queuePathDB = "/data/DeleteShiftById"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r DeleteShiftByIdHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r DeleteShiftByIdHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r DeleteShiftByIdHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r DeleteShiftByIdHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload deleteShiftById

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := deleteShiftByIdQueue{
		DBAssetID: DBassetID,
		ShiftId:   parsedPayload.ShiftId,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
