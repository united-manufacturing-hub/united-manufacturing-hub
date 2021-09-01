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

// getAllItemsInQueue gets all items in the current queue with the exception that it will never get more than 10000 messages at one time
func getAllItemsInQueue(prefix string, pq *goque.PrefixQueue) (itemsInQueue []goque.Item, err error) {
	// TODO: for performance optimization get length and allocate itemsInQueue with make
	for i := 0; i < 10000; i++ { //take the first 10000 messages (if it is not empty, see if)
		item, err2 := pq.Dequeue([]byte(prefix))

		if err2 == goque.ErrEmpty {
			return // abort queue as it is empty
		} else if err2 == goque.ErrOutOfBounds { // TODO: Check why this in the code
			return
		} else if err2 != nil { // Raise error
			err = err2
			zap.S().Errorf("Error Dequeueing", err2)
			return
		}

		//zap.S().Debugf("Adding item", item.ToString())
		itemsInQueue = append(itemsInQueue, *item) // see TODO above
	}
	zap.S().Warnf("Reached maximum level of 10000 messages")

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
