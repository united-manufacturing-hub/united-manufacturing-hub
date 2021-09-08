package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
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

func (r ModifyProducedPieceHandler) Setup() (err error) {
	const queuePathDB = "/data/ModifyProducedPiece"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r ModifyProducedPieceHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r ModifyProducedPieceHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r ModifyProducedPieceHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r ModifyProducedPieceHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

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
