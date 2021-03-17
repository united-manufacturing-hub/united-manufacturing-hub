package main

import (
	"time"

	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

const queuePath = "/data/queue"

type QueueObject struct {
	Object    interface{}
	DBAssetID int
}

func setupQueue() (pq *goque.PrefixQueue, err error) {
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

func reportQueueLength(pg *goque.PrefixQueue) {
	for true {
		zap.S().Infof("Current elements in queue", pg.Length())
		time.Sleep(10 * time.Second)
	}
}
