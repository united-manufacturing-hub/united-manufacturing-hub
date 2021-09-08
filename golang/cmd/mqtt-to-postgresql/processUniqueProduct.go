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
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r UniqueProductHandler) Setup() (err error) {
	const queuePathDB = "/data/UniqueProduct"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r UniqueProductHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r UniqueProductHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r UniqueProductHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r UniqueProductHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload uniqueProduct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	productID, err := GetProductID(DBassetID, parsedPayload.ProductName)
	if err == sql.ErrNoRows {
		zap.S().Errorf("Product does not exist yet", DBassetID, parsedPayload.ProductName)
		go func() {
			time.Sleep(1 * time.Second)
			r.EnqueueMQTT(customerID, location, assetID, payload)
		}()
	} else if err != nil { // never executed
		PQErrorHandling("GetProductID db.QueryRow()", err)
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
