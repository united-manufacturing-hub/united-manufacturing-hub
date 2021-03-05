package main

import "github.com/beeker1121/goque"

type queueObject struct {
	Topic   string
	Message string
}

const queuePath = "/data/queue"

func setupQueue(mode string) (pq goque.PrefixQueue, err error) {
	pq, err := goque.OpenPrefixQueue(queuePath)
	if err != nil {
		zap.S().Errorf("Error opening queue", err)
		return
	}
	return
}

func closeQueue(mode string, pq goque.PrefixQueue) (err error) {

	err := pq.Close()

	if err != nil {
		zap.S().Errorf("Error closing queue", err)
		return
	}

	return
}
