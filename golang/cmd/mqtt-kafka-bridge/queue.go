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
	if !CheckIfNewMessageOrStore(message, topic) {
		zap.S().Debugf("Message already in queue (%s) (%s)", topic, message)
		return
	}
	storeMessageIntoQueue(topic, message, pq)
}

func storeMessageIntoQueue(topic string, message []byte, pq *goque.Queue) {
	zap.S().Debugf("Stored new message in queue (%s) (%s)", topic, message)
	newElement := queueObject{
		Topic:   topic,
		Message: message,
	}
	_, err := pq.EnqueueObject(newElement)
	if err != nil {
		zap.S().Errorf("Error enqueuing %v, %s %v", err, topic, message)
		return
	}
}

func retrieveMessageFromQueue(pq *goque.Queue) (queueObj queueObject, err error, gotItem bool) {
	if pq.Length() == 0 {
		zap.S().Debugf("pq.Length == 0")
		return
	}
	var item *goque.Item
	item, err = pq.Dequeue()
	if err != nil || item == nil {
		if err != nil && err.Error() != "goque: Stack or queue is empty" {
			zap.S().Errorf("Failed to dequeue message: %v", err)
			return queueObj, err, false
		}
		return
	}

	err = item.ToObject(&queueObj)
	if err != nil {
		return
	}
	gotItem = true
	return
}
