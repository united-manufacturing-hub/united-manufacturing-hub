package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type recommendationStruct struct {
	UID                  string
	TimestampMs          uint64 `json:"timestamp_ms"`
	Customer             string
	Location             string
	Asset                string
	RecommendationType   int32
	Enabled              bool
	RecommendationValues string
	DiagnoseTextDE       string
	DiagnoseTextEN       string
	RecommendationTextDE string
	RecommendationTextEN string
}

type RecommendationDataHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r RecommendationDataHandler) Setup() (err error) {
	const queuePathDB = "/data/RecommendationData"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r RecommendationDataHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r RecommendationDataHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r RecommendationDataHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r RecommendationDataHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

	var parsedPayload recommendationStruct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	marshal, err := json.Marshal(parsedPayload)
	if err != nil {
		return
	}

	r.enqueue(marshal, 0)
	return
}
