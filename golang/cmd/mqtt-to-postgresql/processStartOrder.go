package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
	"time"
)

type startOrderQueue struct {
	DBAssetID   uint32
	TimestampMs uint64
	OrderName   string
}
type startOrder struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	OrderName   string `json:"order_id"`
}
type StartOrderHandler struct {
	priorityQueue *goque.PriorityQueue
	shutdown      bool
}

func NewStartOrderHandler() (handler *StartOrderHandler) {
	const queuePathDB = "/data/StartOrder"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &StartOrderHandler{
		priorityQueue: priorityQueue,
		shutdown:      false,
	}
	return
}

func (r StartOrderHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueue.Length() > 0 {
			zap.S().Debugf("StartOrderHandler queue length: %d", r.priorityQueue.Length())
		}
	}
}
func (r StartOrderHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r StartOrderHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		faultyItems, err := storeItemsIntoDatabaseStartOrder(items)
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

func (r StartOrderHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r StartOrderHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.priorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r StartOrderHandler) Shutdown() (err error) {
	zap.S().Warnf("[StartOrderHandler] shutting down, Queue length: %d", r.priorityQueue.Length())
	r.shutdown = true
	time.Sleep(5 * time.Second)
	err = CloseQueue(r.priorityQueue)
	return
}

func (r StartOrderHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	zap.S().Debugf("[StartOrderHandler]")
	var parsedPayload startOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := startOrderQueue{
		TimestampMs: parsedPayload.TimestampMs,
		OrderName:   parsedPayload.OrderName,
		DBAssetID:   DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
