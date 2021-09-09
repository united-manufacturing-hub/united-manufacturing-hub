package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type productTagStringQueue struct {
	DBAssetID   uint32
	TimestampMs uint64 `json:"timestamp_ms"`
	AID         string `json:"AID"`
	Name        string `json:"name"`
	Value       string `json:"value"`
}

type productTagString struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	AID         string `json:"AID"`
	Name        string `json:"name"`
	Value       string `json:"value"`
}

type ProductTagStringHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func NewProductTagStringHandler() (handler *ProductTagStringHandler) {
	const queuePathDB = "/data/ProductTagString"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &ProductTagStringHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r ProductTagStringHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		zap.S().Debugf("ProductTagStringHandler queue length: %d", r.pg.Length())
	}
}
func (r ProductTagStringHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r ProductTagStringHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		faultyItems, err := storeItemsIntoDatabaseProductTagString(items)
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

func (r ProductTagStringHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r ProductTagStringHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r ProductTagStringHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r ProductTagStringHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	zap.S().Debugf("[ProductTagStringHandler]")
	var parsedPayload productTagString

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := productTagStringQueue{
		DBAssetID:   DBassetID,
		TimestampMs: parsedPayload.TimestampMs,
		AID:         parsedPayload.AID,
		Name:        parsedPayload.Name,
		Value:       parsedPayload.Value,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
