package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
	"time"
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
	priorityQueue *goque.PriorityQueue
	shutdown      bool
}

func NewModifyStateHandler() (handler *ModifyStateHandler) {
	const queuePathDB = "/data/ModifyState"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &ModifyStateHandler{
		priorityQueue: priorityQueue,
		shutdown:      false,
	}
	return
}

func (r ModifyStateHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueue.Length() > 0 {
			zap.S().Debugf("ModifyStateHandler queue length: %d", r.priorityQueue.Length())
		}
	}
}
func (r ModifyStateHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r ModifyStateHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		faultyItems, err := modifyStateInDatabase(items)
		if err != nil {
			zap.S().Errorf("err: %s", err)
			if !IsRecoverablePostgresErr(err) {
				ShutdownApplicationGraceful()
				return
			}
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
	if r.priorityQueue.Length() > 0 {
		item, err := r.priorityQueue.Dequeue()
		if err != nil {
			return
		}
		items = append(items, item)

		for true {
			nextItem, err := r.priorityQueue.DequeueByPriority(item.Priority)
			if err != nil {
				break
			}
			items = append(items, nextItem)
		}
	}
	return
}

func (r ModifyStateHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.priorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r ModifyStateHandler) Shutdown() (err error) {
	zap.S().Warnf("[ModifyStateHandler] shutting down, Queue length: %d", r.priorityQueue.Length())
	r.shutdown = true

	err = CloseQueue(r.priorityQueue)
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
