package main

import (
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type queueObject struct {
	Topic   string
	Message []byte
}

const queuePath = "/data/queue"

func setupQueue(mode string) (pq *goque.PrefixQueue, err error) {
	pq, err = goque.OpenPrefixQueue(queuePath)
	if err != nil {
		zap.S().Errorf("Error opening queue", err)
		return
	}
	return
}

func closeQueue(pq *goque.PrefixQueue) (err error) {

	err = pq.Close()

	if err != nil {
		zap.S().Errorf("Error closing queue", err)
		return
	}

	return
}

func storeMessageIntoQueue(topic string, message []byte, mode string, pq *goque.PrefixQueue) {

	newElement := queueObject{
		Topic:   topic,
		Message: message,
	}

	prefix := mode // TODO: add load balancing and multiple queues

	_, err := pq.EnqueueObject([]byte(prefix), newElement)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return
	}
}
