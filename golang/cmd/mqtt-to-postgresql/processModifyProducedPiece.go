package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
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
	pg       *goque.PriorityQueue
	shutdown bool
}

func NewModifyProducedPieceHandler() (handler *ModifyProducedPieceHandler) {
	const queuePathDB = "/data/ModifyProducedPiece"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &ModifyProducedPieceHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r ModifyProducedPieceHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.pg.Length() > 0 {
			zap.S().Debugf("ModifyProducedPieceHandler queue length: %d", r.pg.Length())
		}
	}
}
func (r ModifyProducedPieceHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r ModifyProducedPieceHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		faultyItems, err := modifyInDatabaseModifyCountAndScrap(items)
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

func (r ModifyProducedPieceHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r ModifyProducedPieceHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r ModifyProducedPieceHandler) Shutdown() (err error) {
	zap.S().Warnf("[ModifyProducedPieceHandler] shutting down, Queue length: %d", r.pg.Length())
	r.shutdown = true
	time.Sleep(5 * time.Second)
	err = CloseQueue(r.pg)
	return
}

func (r ModifyProducedPieceHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
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

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := modifyProducesPieceQueue{
		DBAssetID: DBassetID,
		Count:     parsedPayload.Count,
		Scrap:     parsedPayload.Scrap,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}
	r.enqueue(marshal, 0)
	return
}
