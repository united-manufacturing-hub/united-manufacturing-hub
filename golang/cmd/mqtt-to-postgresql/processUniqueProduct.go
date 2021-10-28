package main

import (
	"database/sql"
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
	"time"
)

type uniqueProductQueue struct {
	DBAssetID                  uint32
	BeginTimestampMs           uint64 `json:"begin_timestamp_ms"`
	EndTimestampMs             uint64 `json:"end_timestamp_ms"`
	ProductID                  int32  `json:"productID"`
	IsScrap                    bool   `json:"isScrap"`
	UniqueProductAlternativeID string `json:"uniqueProductAlternativeID"`
}
type uniqueProduct struct {
	BeginTimestampMs           uint64 `json:"begin_timestamp_ms"`
	EndTimestampMs             uint64 `json:"end_timestamp_ms"`
	ProductName                string `json:"productID"`
	IsScrap                    bool   `json:"isScrap"`
	UniqueProductAlternativeID string `json:"uniqueProductAlternativeID"`
}

type UniqueProductHandler struct {
	priorityQueue *goque.PriorityQueue
	shutdown      bool
}

func NewUniqueProductHandler() (handler *UniqueProductHandler) {
	const queuePathDB = "/data/UniqueProduct"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &UniqueProductHandler{
		priorityQueue: priorityQueue,
		shutdown:      false,
	}
	return
}

func (r UniqueProductHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueue.Length() > 0 {
			zap.S().Debugf("UniqueProductHandler queue length: %d", r.priorityQueue.Length())
		}
	}
}
func (r UniqueProductHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r UniqueProductHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		faultyItems, err := storeItemsIntoDatabaseUniqueProduct(items, 0)

		// Empty the array, without de-allocating memory
		items = items[:0]
		for _, faultyItem := range faultyItems {
			var prio uint8
			prio = faultyItem.Priority + 1
			if faultyItem.Priority >= 255 {
				prio = 254
			}
			r.enqueue(faultyItem.Value, prio)
			time.Sleep(time.Duration(100*len(faultyItems)) * time.Millisecond)
		}
		if err != nil {
			zap.S().Errorf("err: %s", err)
			if !IsRecoverablePostgresErr(err) {
				ShutdownApplicationGraceful()
				return
			}
		}
	}
}

func (r UniqueProductHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r UniqueProductHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.priorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r UniqueProductHandler) Shutdown() (err error) {
	zap.S().Warnf("[UniqueProductHandler] shutting down, Queue length: %d", r.priorityQueue.Length())
	r.shutdown = true

	err = CloseQueue(r.priorityQueue)
	return
}

func (r UniqueProductHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	zap.S().Debugf("[UniqueProductHandler]")
	var parsedPayload uniqueProduct

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
	productID, err, success := GetProductID(DBassetID, parsedPayload.ProductName)
	if err == sql.ErrNoRows || !success {
		zap.S().Errorf("Product does not exist yet", DBassetID, parsedPayload.ProductName)
		go func() {
			if r.shutdown {
				storedRawMQTTHandler.EnqueueMQTT(customerID, location, assetID, payload, Prefix.UniqueProduct)
			} else {
				time.Sleep(1 * time.Second)
				r.EnqueueMQTT(customerID, location, assetID, payload)
			}
		}()
		return
	} else if err != nil { // never executed
		PGErrorHandling("GetProductID db.QueryRow()", err)
	}

	newObject := uniqueProductQueue{
		DBAssetID:                  DBassetID,
		BeginTimestampMs:           parsedPayload.BeginTimestampMs,
		EndTimestampMs:             parsedPayload.EndTimestampMs,
		ProductID:                  productID,
		IsScrap:                    parsedPayload.IsScrap,
		UniqueProductAlternativeID: parsedPayload.UniqueProductAlternativeID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
