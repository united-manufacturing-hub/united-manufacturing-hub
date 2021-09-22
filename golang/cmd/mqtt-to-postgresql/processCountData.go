package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
	"time"
)

type countQueue struct {
	DBAssetID   uint32
	Count       uint32
	Scrap       uint32
	TimestampMs uint64
}
type count struct {
	Count       uint32 `json:"count"`
	Scrap       uint32 `json:"scrap"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

type CountHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func NewCountHandler() (handler *CountHandler) {
	const queuePathDB = "/data/Count"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &CountHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r CountHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.pg.Length() > 0 {
			zap.S().Debugf("CountHandler queue length: %d", r.pg.Length())
		}
	}
	zap.S().Debugf("[CountHandler/reportLength] r.shutdown=true")
}
func (r CountHandler) Setup() {
	go r.reportLength()
	go r.process()
}

func (r CountHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		faultyItems, err := storeItemsIntoDatabaseCount(items)
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
	zap.S().Debugf("[CountHandler/process] r.shutdown=true")
}

func (r CountHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r CountHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r CountHandler) Shutdown() (err error) {
	zap.S().Warnf("[CountHandler] shutting down, Queue length: %d", r.pg.Length())
	r.shutdown = true
	time.Sleep(5 * time.Second)
	err = CloseQueue(r.pg)
	return
}

func (r CountHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	var parsedPayload count

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	// this should not happen. Throw a warning message and ignore (= do not try to store in database)
	if parsedPayload.Count <= 0 {
		zap.S().Warnf("count <= 0", customerID, location, assetID, payload, parsedPayload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	newObject := countQueue{
		TimestampMs: parsedPayload.TimestampMs,
		Count:       parsedPayload.Count,
		Scrap:       parsedPayload.Scrap,
		DBAssetID:   DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return
	}
	return
}
