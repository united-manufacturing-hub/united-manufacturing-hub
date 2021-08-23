package main

import (
	"time"

	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

const queuePath = "/data/queue"

const prefixProcessValueFloat64 = "processValueFloat64"
const prefixProcessValue = "processValue"
const prefixCount = "count"
const prefixRecommendation = "recommendation"
const prefixState = "state"
const prefixUniqueProduct = "uniqueProduct"
const prefixScrapCount = "scrapCount"
const prefixAddShift = "addShift"
const prefixUniqueProductScrap = "uniqueProductScrap"
const prefixAddProduct = "addProduct"
const prefixAddOrder = "addOrder"
const prefixStartOrder = "startOrder"
const prefixEndOrder = "endOrder"
const prefixAddMaintenanceActivity = "addMaintenanceActivity"
const prefixProductTag = "productTag"
const prefixProductTagString = "productTagString"
const prefixAddParentToChild = "addParentToChild"

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

func getAllItemsInQueue(prefix string, pq *goque.PrefixQueue) (itemsInQueue []goque.Item, err error) {
	keepRunning := false

	for !keepRunning {
		item, err2 := pq.Dequeue([]byte(prefix))
		if err2 == goque.ErrEmpty || err2 == goque.ErrOutOfBounds {
			return
		} else if err2 != nil {
			err = err2
			zap.S().Errorf("Error Dequeueing", err2)
			return
		}

		//zap.S().Debugf("Adding item", item.ToString())
		itemsInQueue = append(itemsInQueue, *item)

	}

	return
}

func addMultipleItemsToQueue(prefix string, pq *goque.PrefixQueue, itemsInQueue []goque.Item) {
	zap.S().Debugf("addMultipleItemsToQueue")

	for _, item := range itemsInQueue {

		_, err := pq.Enqueue([]byte(prefix), item.Value)
		if err != nil {
			zap.S().Errorf("Error enqueueing", err)
			return
		}

	}

}

func reportQueueLength(pg *goque.PrefixQueue) {
	for true {
		zap.S().Infof("Current elements in queue", pg.Length())
		time.Sleep(10 * time.Second)
	}
}
