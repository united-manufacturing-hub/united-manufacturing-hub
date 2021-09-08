package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type scrapUniqueProductQueue struct {
	DBAssetID uint32
	UID       string
}
type scrapUniqueProduct struct {
	UID string `json:"UID"`
}

type ScrapUniqueProductHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r ScrapUniqueProductHandler) Setup() (err error) {
	const queuePathDB = "/data/ScrapUniqueProduct"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r ScrapUniqueProductHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r ScrapUniqueProductHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r ScrapUniqueProductHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r ScrapUniqueProductHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload scrapUniqueProduct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := scrapUniqueProductQueue{
		UID:       parsedPayload.UID,
		DBAssetID: DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
