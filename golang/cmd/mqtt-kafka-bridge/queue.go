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

func setupQueue(direction string) (pq *goque.Queue, err error) {
	pq, err = goque.OpenQueue(queuePath + "/" + direction)
	if err != nil {
		zap.S().Errorf("Error opening queue", err)
		return
	}
	return
}

func closeQueue(pq *goque.Queue) (err error) {

	err = pq.Close()

	if err != nil {
		zap.S().Errorf("Error closing queue", err)
		return
	}

	return
}

func storeNewMessageIntoQueue(topic string, message []byte, pq *goque.Queue) {
	// Prevents recursion
	if !CheckIfNewMessageOrStore(message) {
		return
	}
	storeMessageIntoQueue(topic, message, pq)
}

func storeMessageIntoQueue(topic string, message []byte, pq *goque.Queue) {
	newElement := queueObject{
		Topic:   topic,
		Message: message,
	}
	_, err := pq.EnqueueObject(newElement)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return
	}
}

func retrieveMessageFromQueue(pq *goque.Queue) (queueObj queueObject, err error) {
	if pq.Length() == 0 {
		return
	}
	var item *goque.Item
	item, err = pq.Dequeue()
	if err != nil || item == nil {
		zap.S().Errorf("Failed to dequeue message", err)
		return
	}

	err = item.ToObject(&queueObj)
	if err != nil {
		return
	}
	return
}
