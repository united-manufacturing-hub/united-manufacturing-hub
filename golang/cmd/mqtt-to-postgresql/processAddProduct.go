package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type addProductQueue struct {
	DBAssetID            uint32
	ProductName          string
	TimePerUnitInSeconds float64
}
type addProduct struct {
	ProductName          string  `json:"product_id"`
	TimePerUnitInSeconds float64 `json:"time_per_unit_in_seconds"`
}

type AddProductHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r AddProductHandler) Setup() (err error) {
	const queuePathDB = "/data/AddProduct"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r AddProductHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r AddProductHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r AddProductHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r AddProductHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload addProduct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := addProductQueue{
		DBAssetID:            DBassetID,
		ProductName:          parsedPayload.ProductName,
		TimePerUnitInSeconds: parsedPayload.TimePerUnitInSeconds,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}
	r.enqueue(marshal, 0)
	return
}
