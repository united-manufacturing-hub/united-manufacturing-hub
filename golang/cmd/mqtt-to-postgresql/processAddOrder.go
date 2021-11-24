package main

import (
	"database/sql"
	"encoding/json"
	"github.com/beeker1121/goque"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

type addOrderQueue struct {
	DBAssetID   uint32
	ProductName string
	OrderName   string
	TargetUnits uint32
	ProductID   int32
}
type addOrder struct {
	ProductName string `json:"product_id"`
	OrderName   string `json:"order_id"`
	TargetUnits uint32 `json:"target_units"`
}

type AddOrderHandler struct {
	priorityQueue *goque.PriorityQueue
	shutdown      bool
}

func NewAddOrderHandler() (handler *AddOrderHandler) {
	const queuePathDB = "/data/AddOrder"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &AddOrderHandler{
		priorityQueue: priorityQueue,
		shutdown:      false,
	}
	return
}

func (r AddOrderHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueue.Length() > 0 {
			zap.S().Debugf("AddOrderHandler queue length: %d", r.priorityQueue.Length())
		}
	}
}
func (r AddOrderHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r AddOrderHandler) process() {
	var items []*goque.PriorityItem
	loopsWithError := int64(0)
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		faultyItems, err := storeItemsIntoDatabaseAddOrder(items, 0)

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

func (r AddOrderHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r AddOrderHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.priorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r AddOrderHandler) Shutdown() (err error) {
	zap.S().Warnf("[AddOrderHandler] shutting down, Queue length: %d", r.priorityQueue.Length())
	r.shutdown = true

	err = CloseQueue(r.priorityQueue)
	return
}

func (r AddOrderHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte, recursionDepth int64) {
	zap.S().Debugf("[AddOrderHandler]")
	var parsedPayload addOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID, success := GetAssetID(customerID, location, assetID, 0)
	var productID int32
	if success {
		productID, err, success = GetProductID(DBassetID, parsedPayload.ProductName, 0)
	}

	if err == sql.ErrNoRows || !success {
		zap.S().Errorf("Failed to AddOrder", DBassetID, parsedPayload.ProductName, parsedPayload.OrderName)
		go func() {
			if r.shutdown {
				storedRawMQTTHandler.EnqueueMQTT(customerID, location, assetID, payload, Prefix.AddOrder, recursionDepth+1)
			} else {
				internal.SleepBackedOff(recursionDepth, 10000*time.Nanosecond, 1000*time.Millisecond)
				r.EnqueueMQTT(customerID, location, assetID, payload, recursionDepth+1)
			}
		}()
		return
	} else if err != nil { // never executed
		PGErrorHandling("GetProductID db.QueryRow()", err)
	}

	newObject := addOrderQueue{
		DBAssetID:   DBassetID,
		ProductName: parsedPayload.ProductName,
		OrderName:   parsedPayload.OrderName,
		TargetUnits: parsedPayload.TargetUnits,
		ProductID:   productID,
	}
	if !ValidateStruct(newObject) {
		zap.S().Errorf("Failed to validate struct of type addOrderQueue", newObject)
		return
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
