package main

import (
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
	"time"
)

type XHandler struct {
	priorityQueue *goque.PriorityQueue
	shutdown      bool
}

func NewXHandler() (handler *XHandler) {
	const queuePathDB = "/data/X"
	var priorityQueue *goque.PriorityQueue
	var err error
	priorityQueue, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		zap.S().Errorf("err: %s", err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &XHandler{
		priorityQueue: priorityQueue,
		shutdown:      false,
	}
	return
}

func (r XHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueue.Length() > 0 {
			zap.S().Debugf("XHandler queue length: %d", r.priorityQueue.Length())
		}
	}
}
func (r XHandler) Setup() {
	go r.reportLength()
	go r.process()
}
func (r XHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r XHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.priorityQueue.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r XHandler) Shutdown() (err error) {
	zap.S().Warnf("[XHandler] shutting down, Queue length: %d", r.priorityQueue.Length())
	r.shutdown = true

	err = CloseQueue(r.priorityQueue)
	return
}

func (r XHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte, recursionDepth int64) {
	zap.S().Debugf("[XHandler]")
	var marshal []byte

	r.enqueue(marshal, 0)
	return
}
