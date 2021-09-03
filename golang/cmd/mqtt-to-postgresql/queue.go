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

func processQueue(pg *goque.PriorityQueue) {
	zap.S().Infof("Starting new processQueue worker")
	for !shuttingDown {
		if pg.Length() == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		item, prio, err := GetNextItemInQueue(pg)
		if err != nil {
			err := reInsertItemOnFailure(pg, item.prefix, item.Object, prio)
			if err != nil {
				zap.S().Errorf("Failed to re-queue failed item, shutting down !", err)
				ShutdownApplicationGraceful()
				return
			}
		}

		i := goque.Item{}
		i.Value = item.Object

		var processError error
		switch item.prefix {
		case Prefix.ProcessValueFloat64:
			processError = storeItemsIntoDatabaseProcessValueFloat64(i)
		case Prefix.ProcessValue:
			processError = storeItemsIntoDatabaseProcessValue(i)
		case Prefix.Count:
			processError = storeItemsIntoDatabaseCount(i)
		case Prefix.Recommendation:
			processError = storeItemsIntoDatabaseRecommendation(i)
		case Prefix.State:
			processError = storeItemsIntoDatabaseState(i)
		case Prefix.UniqueProduct:
			processError = storeItemsIntoDatabaseUniqueProduct(i)
		case Prefix.ScrapCount:
			processError = storeItemsIntoDatabaseScrapCount(i)
		case Prefix.AddShift:
			processError = storeItemsIntoDatabaseShift(i)
		case Prefix.UniqueProductScrap:
			processError = storeItemsIntoDatabaseUniqueProductScrap(i)
		case Prefix.AddProduct:
			processError = storeItemsIntoDatabaseAddProduct(i)
		case Prefix.AddOrder:
			processError = storeItemsIntoDatabaseAddOrder(i)
		case Prefix.StartOrder:
			processError = storeItemsIntoDatabaseStartOrder(i)
		case Prefix.EndOrder:
			processError = storeItemsIntoDatabaseEndOrder(i)
		case Prefix.AddMaintenanceActivity:
			processError = storeItemsIntoDatabaseAddMaintenanceActivity(i)
		case Prefix.ProductTag:
			processError = storeItemsIntoDatabaseProductTag(i)
		case Prefix.ProductTagString:
			processError = storeItemsIntoDatabaseProductTagString(i)
		case Prefix.AddParentToChild:
			processError = storeItemsIntoDatabaseAddParentToChild(i)
		case Prefix.ModifyState:
			processError = modifyStateInDatabase(i)
		case Prefix.ModifyProducesPieces:
			processError = modifyInDatabaseModifyCountAndScrap(i)
		case Prefix.DeleteShiftById:
			processError = deleteShiftInDatabaseById(i)
		case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
			processError = deleteShiftInDatabaseByAssetIdAndTimestamp(i)
		}

		if processError != nil {
			zap.S().Warnf("processError: %s", processError)
			err := reInsertItemOnFailure(pg, item.prefix, item.Object, prio)
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
	Object []byte
	prefix string
}

func GetNextItemInQueue(pg *goque.PriorityQueue) (item QueueObject, prio uint8, err error) {
	priorityItem, err := pg.Dequeue()
	if err != nil {
		return QueueObject{}, 0, err
	}
	prio = priorityItem.Priority
	err = priorityItem.ToObject(&item)
	if err != nil {
		return QueueObject{}, 0, err
	}
	return
}

func reInsertItemOnFailure(pg *goque.PriorityQueue, prefix string, object []byte, prio uint8) (err error) {
	if prio < 255 {
		prio += 1
		err = addItemWithPrioToQueue(pg, prefix, object, prio)
		if err != nil {
			return
		}
	} else {
		zap.S().Errorf("Item %s for prefix %s failed 255 times, waiting before requing")
		if shuttingDown {
			zap.S().Errorf("Application is shuting down, while having corrupt item !")
		} else {
			time.Sleep(10 * time.Second)
		}
		// Let this fail once, then put it on hold, to avoid blocking the rest
		// Also gets the lowest priority to not block queue
		err = addItemWithPrioToQueue(pg, prefix, object, 255)
		if err != nil {
			return
		}
	}
	return
}

func addItemWithPrioToQueue(pq *goque.PriorityQueue, prefix string, item []byte, priority uint8) (err error) {
	qo := QueueObject{prefix: prefix, Object: item}
	_, err = pq.EnqueueObject(priority, qo)
	return
}

func addNewItemToQueue(pq *goque.PriorityQueue, prefix string, item []byte) (err error) {
	err = addItemWithPrioToQueue(pq, prefix, item, 0)
	return
}

func addNewItemsToQueue(pq *goque.PriorityQueue, prefix string, items [][]byte) (err error) {
	zap.S().Debugf("addNewItemsToQueue")
	for _, item := range items {
		err = addNewItemToQueue(pq, prefix, item)
		if err != nil {
			return
		}
	}
	return
}

func reportQueueLength(pg *goque.PriorityQueue) {
	for true {
		zap.S().Infof("Current elements in queue", pg.Length())
		time.Sleep(10 * time.Second)
	}
}
