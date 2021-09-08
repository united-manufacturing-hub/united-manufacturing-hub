package main

import (
	"database/sql"
	"encoding/json"
	"github.com/beeker1121/goque"
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
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r AddOrderHandler) Setup() (err error) {
	const queuePathDB = "/data/AddOrder"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r AddOrderHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r AddOrderHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r AddOrderHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r AddOrderHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload addOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	productID, err := GetProductID(DBassetID, parsedPayload.ProductName)
	if err == sql.ErrNoRows {
		zap.S().Errorf("Product does not exist yet", DBassetID, parsedPayload.ProductName, parsedPayload.OrderName)
		go func() {
			time.Sleep(1 * time.Second)
			r.EnqueueMQTT(customerID, location, assetID, payload)
		}()
		return
	} else if err != nil { // never executed
		PQErrorHandling("GetProductID db.QueryRow()", err)
	}

	newObject := addOrderQueue{
		DBAssetID:   DBassetID,
		ProductName: parsedPayload.ProductName,
		OrderName:   parsedPayload.OrderName,
		TargetUnits: parsedPayload.TargetUnits,
		ProductID:   productID,
	}
	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
