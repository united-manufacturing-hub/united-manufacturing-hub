package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
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
	pg       *goque.PriorityQueue
	shutdown bool
}

func NewDeleteShiftByAssetIdAndBeginTimestampHandler() (handler *DeleteShiftByAssetIdAndBeginTimestampHandler) {
	const queuePathDB = "/data/DeleteShiftByAssetIdAndBeginTimestamp"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &DeleteShiftByAssetIdAndBeginTimestampHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.pg.Length() > 0 {
			zap.S().Debugf("DeleteShiftByAssetIdAndBeginTimestampHandler queue length: %d", r.pg.Length())
		}
	}
}
func (r DeleteShiftByAssetIdAndBeginTimestampHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r DeleteShiftByAssetIdAndBeginTimestampHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		faultyItems, err := deleteShiftInDatabaseByAssetIdAndTimestamp(items)
		if err != nil {
			zap.S().Errorf("err: %s", err)
			ShutdownApplicationGraceful()
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

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) Shutdown() (err error) {
	zap.S().Warnf("[DeleteShiftByAssetIdAndBeginTimestampHandler] shutting down, Queue length: %d", r.pg.Length())
	r.shutdown = true
	time.Sleep(5 * time.Second)
	err = CloseQueue(r.pg)
	return
}

func (r DeleteShiftByAssetIdAndBeginTimestampHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	zap.S().Debugf("[DeleteShiftByAssetIdAndBeginTimestampHandler]")
	var parsedPayload deleteShiftByAssetIdAndBeginTimestamp

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := deleteShiftByAssetIdAndBeginTimestampQueue{
		DBAssetID:        DBassetID,
		BeginTimeStampMs: parsedPayload.BeginTimeStampMs,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
