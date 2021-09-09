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

func NewMaintenanceActivityHandler() (handler *MaintenanceActivityHandler) {
	const queuePathDB = "/data/MaintenanceActivity"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(pg)
	handler = &MaintenanceActivityHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r MaintenanceActivityHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		faultyItems, err := storeItemsIntoDatabaseAddMaintenanceActivity(items)
		if err != nil {
			return
		}
		// Empty the array, without de-allocating memory
		items = items[:0]
		for _, faultyItem := range faultyItems {
			var prio uint8
			prio = faultyItem.Priority + 1
			if faultyItem.Priority >= 255 {
				prio = 254
			}
			r.enqueue(faultyItem.Value, prio)
		}
	}
}

func (r MaintenanceActivityHandler) dequeue() (items []*goque.PriorityItem) {
	if r.pg.Length() > 0 {
		item, err := r.pg.Dequeue()
		if err != nil {
			return
		}
		items = append(items, item)

		for true {
			nextItem, err := r.pg.DequeueByPriority(item.Priority)
			if err != nil {
				break
			}
			items = append(items, nextItem)
		}
	}
	return
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
	zap.S().Debugf("[MaintenanceActivityHandler]")
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
