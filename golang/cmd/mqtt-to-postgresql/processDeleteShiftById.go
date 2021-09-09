package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
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
	pg       *goque.PriorityQueue
	shutdown bool
}

func NewDeleteShiftByIdHandler() (handler *DeleteShiftByIdHandler) {
	const queuePathDB = "/data/DeleteShiftById"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &DeleteShiftByIdHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r DeleteShiftByIdHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		zap.S().Debugf("DeleteShiftByIdHandler queue length: %d", r.pg.Length())
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
		faultyItems, err := deleteShiftInDatabaseById(items)
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

func (r DeleteShiftByIdHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r DeleteShiftByIdHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r DeleteShiftByIdHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
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

	DBassetID := GetAssetID(customerID, location, assetID)
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
