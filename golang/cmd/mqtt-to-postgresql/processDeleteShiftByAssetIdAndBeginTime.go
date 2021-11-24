package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

type deleteShiftByAssetIdAndBeginTimestampQueue struct {
	DBAssetID        uint32
	BeginTimeStampMs uint32 `json:"begin_time_stamp"`
}

type deleteShiftByAssetIdAndBeginTimestamp struct {
	BeginTimeStampMs uint32 `json:"begin_time_stamp"`
}

type DeleteShiftByAssetIdAndBeginTimestampHandler struct {
	priorityQueue *goque.PriorityQueue
	shutdown      bool
}

func NewDeleteShiftByAssetIdAndBeginTimestampHandler() (handler *DeleteShiftByAssetIdAndBeginTimestampHandler) {
	const queuePathDB = "/data/DeleteShiftByAssetIdAndBeginTimestamp"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &DeleteShiftByAssetIdAndBeginTimestampHandler{
		priorityQueue: priorityQueue,
		shutdown:      false,
	}
	return
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueue.Length() > 0 {
			zap.S().Debugf("DeleteShiftByAssetIdAndBeginTimestampHandler queue length: %d", r.priorityQueue.Length())
		}
	}
}
func (r DeleteShiftByAssetIdAndBeginTimestampHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r DeleteShiftByAssetIdAndBeginTimestampHandler) process() {
	var items []*goque.PriorityItem
	loopsWithError := int64(0)
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		faultyItems, err := deleteShiftInDatabaseByAssetIdAndTimestamp(items, 0)

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

		if err != nil {
			zap.S().Errorf("err: %s", err)
			switch GetPostgresErrorRecoveryOptions(err) {
			case Unrecoverable:
				ShutdownApplicationGraceful()
			}
		}

		if err != nil || len(faultyItems) > 0 {
			loopsWithError += 1
		} else {
			loopsWithError = 0
		}

		internal.SleepBackedOff(loopsWithError, 10000*time.Nanosecond, 1000*time.Millisecond)
	}
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.priorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) Shutdown() (err error) {
	zap.S().Warnf("[DeleteShiftByAssetIdAndBeginTimestampHandler] shutting down, Queue length: %d", r.priorityQueue.Length())
	r.shutdown = true

	err = CloseQueue(r.priorityQueue)
	return
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte, recursionDepth int64) {
	zap.S().Debugf("[DeleteShiftByAssetIdAndBeginTimestampHandler]")
	var parsedPayload deleteShiftByAssetIdAndBeginTimestamp

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID, success := GetAssetID(customerID, location, assetID, 0)
	if !success {
		go func() {
			if r.shutdown {
				storedRawMQTTHandler.EnqueueMQTT(customerID, location, assetID, payload, Prefix.AddOrder, recursionDepth+1)
			} else {
				internal.SleepBackedOff(recursionDepth, 10000*time.Nanosecond, 1000*time.Millisecond)
				r.EnqueueMQTT(customerID, location, assetID, payload, recursionDepth+1)
			}
		}()
		return
	}
	newObject := deleteShiftByAssetIdAndBeginTimestampQueue{
		DBAssetID:        DBassetID,
		BeginTimeStampMs: parsedPayload.BeginTimeStampMs,
	}
	if !ValidateStruct(newObject) {
		zap.S().Errorf("Failed to validate struct of type deleteShiftByAssetIdAndBeginTimestampQueue", newObject)
		return
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
