package main

import (
	"time"

	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

func SetupQueue(queuePath string) (pq *goque.PriorityQueue, err error) {

	pq, err = goque.OpenPriorityQueue(queuePath, goque.ASC)
	if err != nil {
		zap.S().Errorf("Error opening queue", err)
		return
	}
	return
}

func CloseQueue(pq *goque.PriorityQueue) (err error) {

	err = pq.Close()
	if err != nil {
		zap.S().Errorf("Error closing queue", err)
		return
	}

	return
}

// reportQueueLength prints the current Queue length
func reportQueueLength(priorityQueue *goque.PriorityQueue) {
	for true {
		zap.S().Infof("Current elements in queue: %d", priorityQueue.Length())
		time.Sleep(10 * time.Second)
	}
}
