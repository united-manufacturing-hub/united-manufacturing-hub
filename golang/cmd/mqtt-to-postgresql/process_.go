package main

import (
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type XHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func NewXHandler() (handler *XHandler) {
	const queuePathDB = "/data/X"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		ShutdownApplicationGraceful()
		panic("Failed to setup queue, exiting !")
	}

	handler = &XHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r XHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r XHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r XHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r XHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	zap.S().Debugf("[XHandler]")
	var marshal []byte

	r.enqueue(marshal, 0)
	return
}
