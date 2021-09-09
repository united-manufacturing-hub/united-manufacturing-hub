package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
	"time"
)

type addShiftQueue struct {
	DBAssetID      uint32
	TimestampMs    uint64
	TimestampMsEnd uint64
}
type addShift struct {
	TimestampMs    uint64 `json:"timestamp_ms"`
	TimestampMsEnd uint64 `json:"timestamp_ms_end"`
}

type AddShiftHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func NewAddShiftHandler() (handler *AddShiftHandler) {
	const queuePathDB = "/data/AddShift"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &AddShiftHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r AddShiftHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.pg.Length() > 0 {
			zap.S().Debugf("AddShiftHandler queue length: %d", r.pg.Length())
		}
	}
}
func (r AddShiftHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r AddShiftHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(10 * time.Millisecond)
		}
		faultyItems, err := storeItemsIntoDatabaseShift(items)
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

func (r AddShiftHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r AddShiftHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r AddShiftHandler) Shutdown() (err error) {
	zap.S().Warnf("[AddShiftHandler] shutting down !")
	r.shutdown = true
	time.Sleep(5 * time.Second)
	err = CloseQueue(r.pg)
	return
}

func (r AddShiftHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	zap.S().Debugf("[AddShiftHandler]")
	var parsedPayload addShift

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := addShiftQueue{
		TimestampMs:    parsedPayload.TimestampMs,
		TimestampMsEnd: parsedPayload.TimestampMsEnd,
		DBAssetID:      DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
