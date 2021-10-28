package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
	"math"
	"time"
)

type deleteShiftByIdQueue struct {
	DBAssetID uint32
	ShiftId   uint32 `json:"shift_id"`
}

type deleteShiftById struct {
	ShiftId uint32 `json:"shift_id"`
}

type DeleteShiftByIdHandler struct {
	priorityQueue *goque.PriorityQueue
	shutdown      bool
}

func NewDeleteShiftByIdHandler() (handler *DeleteShiftByIdHandler) {
	const queuePathDB = "/data/DeleteShiftById"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &DeleteShiftByIdHandler{
		priorityQueue: priorityQueue,
		shutdown:      false,
	}
	return
}

func (r DeleteShiftByIdHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueue.Length() > 0 {
			zap.S().Debugf("DeleteShiftByIdHandler queue length: %d", r.priorityQueue.Length())
		}
	}
}
func (r DeleteShiftByIdHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r DeleteShiftByIdHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		faultyItems, err := deleteShiftInDatabaseById(items, 0)

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
		time.Sleep(time.Duration(math.Min(float64(100*len(faultyItems)), 1000)) * time.Millisecond)
		if err != nil {
			zap.S().Errorf("err: %s", err)
			if !IsRecoverablePostgresErr(err) {
				ShutdownApplicationGraceful()
				return
			}
		}
	}
}

func (r DeleteShiftByIdHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r DeleteShiftByIdHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.priorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r DeleteShiftByIdHandler) Shutdown() (err error) {
	zap.S().Warnf("[DeleteShiftByIdHandler] shutting down, Queue length: %d", r.priorityQueue.Length())
	r.shutdown = true

	err = CloseQueue(r.priorityQueue)
	return
}

func (r DeleteShiftByIdHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	zap.S().Debugf("[DeleteShiftByIdHandler]")
	var parsedPayload deleteShiftById

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID, success := GetAssetID(customerID, location, assetID)
	if !success {
		go func() {
			if r.shutdown {
				storedRawMQTTHandler.EnqueueMQTT(customerID, location, assetID, payload, Prefix.AddOrder)
			} else {
				time.Sleep(1 * time.Second)
				r.EnqueueMQTT(customerID, location, assetID, payload)
			}
		}()
		return
	}
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
