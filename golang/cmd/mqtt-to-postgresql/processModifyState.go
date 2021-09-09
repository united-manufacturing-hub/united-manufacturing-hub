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

func NewModifyStateHandler() (handler *ModifyStateHandler) {
	const queuePathDB = "/data/ModifyState"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(pg)
	handler = &ModifyStateHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r ModifyStateHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		faultyItems, err := modifyStateInDatabase(items)
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

func (r ModifyStateHandler) dequeue() (items []*goque.PriorityItem) {
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
	zap.S().Debugf("[ModifyStateHandler]")
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
