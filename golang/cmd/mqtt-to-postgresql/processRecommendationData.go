package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
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
	priorityQueue *goque.PriorityQueue
	shutdown      bool
}

func NewRecommendationDataHandler() (handler *RecommendationDataHandler) {
	const queuePathDB = "/data/RecommendationData"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &RecommendationDataHandler{
		priorityQueue: priorityQueue,
		shutdown:      false,
	}
	return
}

func (r RecommendationDataHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueue.Length() > 0 {
			zap.S().Debugf("RecommendationDataHandler queue length: %d", r.priorityQueue.Length())
		}
	}
}
func (r RecommendationDataHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r RecommendationDataHandler) process() {
	var items []*goque.PriorityItem
	loopsWithError := int64(0)
	for !r.shutdown {
		items = r.dequeue()
		if len(items) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		faultyItems, err := storeItemsIntoDatabaseRecommendation(items, 0)

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

func (r RecommendationDataHandler) dequeue() (items []*goque.PriorityItem) {
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

func (r RecommendationDataHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.priorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r RecommendationDataHandler) Shutdown() (err error) {
	zap.S().Warnf("[RecommendationDataHandler] shutting down, Queue length: %d", r.priorityQueue.Length())
	r.shutdown = true

	err = CloseQueue(r.priorityQueue)
	return
}

func (r RecommendationDataHandler) EnqueueMQTT(_ string, _ string, _ string, payload []byte, recursionDepth int64) {
	zap.S().Debugf("[RecommendationDataHandler]")
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
