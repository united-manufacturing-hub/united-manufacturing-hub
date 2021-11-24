package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

type modifyProducesPieceQueue struct {
	DBAssetID uint32
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Count int32 `json:"count"`
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Scrap int32 `json:"scrap"`
}

type modifyProducesPiece struct {
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Count int32 `json:"count"`
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Scrap int32 `json:"scrap"`
}

type ModifyProducedPieceHandler struct {
	priorityQueue *goque.PriorityQueue
	shutdown      bool
}

func NewModifyProducedPieceHandler() (handler *ModifyProducedPieceHandler) {
	const queuePathDB = "/data/ModifyProducedPiece"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &ModifyProducedPieceHandler{
		priorityQueue: priorityQueue,
		shutdown:      false,
	}
	return
}

func (r ModifyProducedPieceHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueue.Length() > 0 {
			zap.S().Debugf("ModifyProducedPieceHandler queue length: %d", r.priorityQueue.Length())
		}
	}
}
func (r ModifyProducedPieceHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r ModifyProducedPieceHandler) process() {
	var items []*goque.PriorityItem
	loopsWithError := int64(0)
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		faultyItems, err := modifyInDatabaseModifyCountAndScrap(items, 0)

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

func (r ModifyProducedPieceHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r ModifyProducedPieceHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.priorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r ModifyProducedPieceHandler) Shutdown() (err error) {
	zap.S().Warnf("[ModifyProducedPieceHandler] shutting down, Queue length: %d", r.priorityQueue.Length())
	r.shutdown = true

	err = CloseQueue(r.priorityQueue)
	return
}

func (r ModifyProducedPieceHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte, recursionDepth int64) {
	zap.S().Debugf("[ModifyProducedPieceHandler]")
	// pt.Scrap is -1, if not modified by user
	// pt.Count is -1, if not modified by user
	parsedPayload := modifyProducesPiece{
		Count: -1,
		Scrap: -1,
	}

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
	newObject := modifyProducesPieceQueue{
		DBAssetID: DBassetID,
		Count:     parsedPayload.Count,
		Scrap:     parsedPayload.Scrap,
	}
	if !ValidateStruct(newObject) {
		zap.S().Errorf("Failed to validate struct of type modifyProducesPieceQueue", newObject)
		return
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}
	r.enqueue(marshal, 0)
	return
}
