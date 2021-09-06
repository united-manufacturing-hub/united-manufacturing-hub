package main

import (
	"time"

	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

const queuePath = "/data/queue"

func setupQueue() (pq *goque.PriorityQueue, err error) {
	pq, err = goque.OpenPriorityQueue(queuePath, goque.ASC)
	if err != nil {
		zap.S().Errorf("Error opening queue", err)
		return
	}
	return
}

func closeQueue(pq *goque.PriorityQueue) (err error) {
	err = pq.Close()
	if err != nil {
		zap.S().Errorf("Error closing queue", err)
		return
	}

	return
}

// processQueue get's item from queue and processes it, depending on prefix
func processQueue(pg *goque.PriorityQueue) {
	zap.S().Infof("Starting new processQueue worker")
	for !shuttingDown {
		if pg.Length() == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		zap.S().Debugf("pg.Length() != 0")
		item, prio, err := GetNextItemInQueue(pg)
		if err != nil {
			err := ReInsertItemOnFailure(pg, item, prio)
			if err != nil {
				zap.S().Errorf("Failed to re-queue failed item, shutting down !", err)
				ShutdownApplicationGraceful()
				return
			}
		}
		zap.S().Debugf("Got an item ! Prefix: %s | Value: %s", item.Prefix, item)

		var processError error
		switch item.Prefix {
		case Prefix.ProcessValueFloat64:
			processError = storeItemsIntoDatabaseProcessValueFloat64(item)
		case Prefix.ProcessValue:
			processError = storeItemsIntoDatabaseProcessValue(item)
		case Prefix.Count:
			processError = storeItemsIntoDatabaseCount(item)
		case Prefix.Recommendation:
			processError = storeItemsIntoDatabaseRecommendation(item)
		case Prefix.State:
			processError = storeItemsIntoDatabaseState(item)
		case Prefix.UniqueProduct:
			processError = storeItemsIntoDatabaseUniqueProduct(item)
		case Prefix.ScrapCount:
			processError = storeItemsIntoDatabaseScrapCount(item)
		case Prefix.AddShift:
			processError = storeItemsIntoDatabaseShift(item)
		case Prefix.UniqueProductScrap:
			processError = storeItemsIntoDatabaseUniqueProductScrap(item)
		case Prefix.AddProduct:
			processError = storeItemsIntoDatabaseAddProduct(item)
		case Prefix.AddOrder:
			processError = storeItemsIntoDatabaseAddOrder(item)
		case Prefix.StartOrder:
			processError = storeItemsIntoDatabaseStartOrder(item)
		case Prefix.EndOrder:
			processError = storeItemsIntoDatabaseEndOrder(item)
		case Prefix.AddMaintenanceActivity:
			processError = storeItemsIntoDatabaseAddMaintenanceActivity(item)
		case Prefix.ProductTag:
			processError = storeItemsIntoDatabaseProductTag(item)
		case Prefix.ProductTagString:
			processError = storeItemsIntoDatabaseProductTagString(item)
		case Prefix.AddParentToChild:
			processError = storeItemsIntoDatabaseAddParentToChild(item)
		case Prefix.ModifyState:
			processError = modifyStateInDatabase(item)
		case Prefix.ModifyProducesPieces:
			processError = modifyInDatabaseModifyCountAndScrap(item)
		case Prefix.DeleteShiftById:
			processError = deleteShiftInDatabaseById(item)
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
			processError = deleteShiftInDatabaseByAssetIdAndTimestamp(item)
		default:
			zap.S().Errorf("GQ Item with invalid Prefix! ", item.Prefix)
		}
		if processError != nil {
			zap.S().Warnf("processError: %s", processError)
			err := ReInsertItemOnFailure(pg, item, prio)
			if err != nil {
				zap.S().Errorf("Failed to re-queue failed item, shutting down !", err)
				ShutdownApplicationGraceful()
				return
			}
		}

	}
	zap.S().Infof("processQueue worker shutting down")
}

type QueueObject struct {
	Prefix  string
	Payload []byte
}

// ReInsertItemOnFailure This functions re-inserts an item with lowered priority
func ReInsertItemOnFailure(pg *goque.PriorityQueue, object QueueObject, prio uint8) (err error) {
	if prio < 255 {
		prio += 1
		err = addItemWithPriorityToQueue(pg, object, prio)
		if err != nil {
			return
		}
	} else {
		zap.S().Errorf("Item %s for Prefix %s failed 255 times, waiting before requing")
		if shuttingDown {
			zap.S().Errorf("Application is shuting down, while having corrupt item !")
		} else {
			time.Sleep(10 * time.Second)
		}
		// Let this fail once, then put it on hold, to avoid blocking the rest
		// Also gets the lowest priority to not block queue
		err = addItemWithPriorityToQueue(pg, object, 255)
		if err != nil {
			return
		}
	}
	return
}

// GetNextItemInQueue retrieves the next item in queue
func GetNextItemInQueue(pg *goque.PriorityQueue) (item QueueObject, prio uint8, err error) {
	priorityItem, err := pg.Dequeue()
	if err != nil {
		return QueueObject{}, 0, err
	}
	zap.S().Debugf("GetNextItemInQueue", priorityItem)
	prio = priorityItem.Priority
	err = priorityItem.ToObjectFromJSON(&item)
	if err != nil {
		return QueueObject{}, 0, err
	}
	return
}

// addItemWithPriorityToQueue adds an item with given priority to queue
func addItemWithPriorityToQueue(pq *goque.PriorityQueue, item QueueObject, priority uint8) (err error) {
	zap.S().Debugf("addItemWithPriorityToQueue", item.Prefix, item.Payload, priority)
	var pi *goque.PriorityItem
	pi, err = pq.EnqueueObjectAsJSON(priority, item)
	zap.S().Debugf("addItemWithPriorityToQueue", pi)
	return
}

// addNewItemToQueue adds an item with 0 priority to queue
func addNewItemToQueue(pq *goque.PriorityQueue, payloadType string, payload []byte) (err error) {
	zap.S().Debugf("addNewItemToQueue", payload)
	item := QueueObject{
		Prefix:  payloadType,
		Payload: payload,
	}
	err = addItemWithPriorityToQueue(pq, item, 0)
	return
}

// reportQueueLength prints the current Queue length
func reportQueueLength(pg *goque.PriorityQueue) {
	for true {
		zap.S().Infof("Current elements in queue: %d", pg.Length())
		if pg.Length() > 0 {
			peek, err := pg.Peek()
			if err != nil {
				continue
			}
			var qo QueueObject
			zap.S().Infof("%s", peek.ToObject(&qo))
		}
		time.Sleep(10 * time.Second)
	}
}
