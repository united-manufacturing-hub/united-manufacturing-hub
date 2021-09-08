package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type addMaintenanceActivityQueue struct {
	DBAssetID     uint32
	TimestampMs   uint64
	ComponentName string
	Activity      int32
	ComponentID   int32
}
type addMaintenanceActivity struct {
	TimestampMs   uint64 `json:"timestamp_ms"`
	ComponentName string `json:"component"`
	Activity      int32  `json:"activity"`
}

type MaintenanceActivityHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r MaintenanceActivityHandler) Setup() (err error) {
	const queuePathDB = "/data/state/X"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r MaintenanceActivityHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r MaintenanceActivityHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r MaintenanceActivityHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r MaintenanceActivityHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload addMaintenanceActivity

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	componentID := GetComponentID(DBassetID, parsedPayload.ComponentName)
	if componentID == 0 {
		zap.S().Errorf("GetComponentID failed")
		return
	}

	newObject := addMaintenanceActivityQueue{
		DBAssetID:     DBassetID,
		TimestampMs:   parsedPayload.TimestampMs,
		ComponentName: parsedPayload.ComponentName,
		ComponentID:   componentID,
		Activity:      parsedPayload.Activity,
	}
	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
