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

		var items []QueueObject
		items, err = GetNextItemsWithPrio(pg, prio)
		if err != nil {
			err := ReInsertItemOnFailure(pg, item, prio)
			if err != nil {
				zap.S().Errorf("Failed to re-queue failed item, shutting down !", err)
				ShutdownApplicationGraceful()
				return
			}
		}
		items = append(items, item)

		var ListstoreItemsIntoDatabaseProcessValueFloat64 []QueueObject
		var ListstoreItemsIntoDatabaseProcessValue []QueueObject
		var ListstoreItemsIntoDatabaseProcessValueString []QueueObject
		var ListstoreItemsIntoDatabaseCount []QueueObject
		var ListstoreItemsIntoDatabaseRecommendation []QueueObject
		var ListstoreItemsIntoDatabaseState []QueueObject
		var ListstoreItemsIntoDatabaseUniqueProduct []QueueObject
		var ListstoreItemsIntoDatabaseScrapCount []QueueObject
		var ListstoreItemsIntoDatabaseShift []QueueObject
		var ListstoreItemsIntoDatabaseUniqueProductScrap []QueueObject
		var ListstoreItemsIntoDatabaseAddProduct []QueueObject
		var ListstoreItemsIntoDatabaseAddOrder []QueueObject
		var ListstoreItemsIntoDatabaseStartOrder []QueueObject
		var ListstoreItemsIntoDatabaseEndOrder []QueueObject
		var ListstoreItemsIntoDatabaseAddMaintenanceActivity []QueueObject
		var ListstoreItemsIntoDatabaseProductTag []QueueObject
		var ListstoreItemsIntoDatabaseProductTagString []QueueObject
		var ListstoreItemsIntoDatabaseAddParentToChild []QueueObject
		var ListmodifyStateInDatabase []QueueObject
		var ListmodifyInDatabaseModifyCountAndScrap []QueueObject
		var ListdeleteShiftInDatabaseById []QueueObject
		var ListdeleteShiftInDatabaseByAssetIdAndTimestamp []QueueObject

		for _, item := range items {
			switch item.Prefix {
			case Prefix.ProcessValueFloat64:
				ListstoreItemsIntoDatabaseProcessValueFloat64 = append(ListstoreItemsIntoDatabaseProcessValueFloat64, item)
			case Prefix.ProcessValue:
				ListstoreItemsIntoDatabaseProcessValue = append(ListstoreItemsIntoDatabaseProcessValue, item)
			case Prefix.ProcessValueString:
				ListstoreItemsIntoDatabaseProcessValueString = append(ListstoreItemsIntoDatabaseProcessValueString, item)
			case Prefix.Count:
				ListstoreItemsIntoDatabaseCount = append(ListstoreItemsIntoDatabaseCount, item)
			case Prefix.Recommendation:
				ListstoreItemsIntoDatabaseRecommendation = append(ListstoreItemsIntoDatabaseRecommendation, item)
			case Prefix.State:
				ListstoreItemsIntoDatabaseState = append(ListstoreItemsIntoDatabaseState, item)
			case Prefix.UniqueProduct:
				ListstoreItemsIntoDatabaseUniqueProduct = append(ListstoreItemsIntoDatabaseUniqueProduct, item)
			case Prefix.ScrapCount:
				ListstoreItemsIntoDatabaseScrapCount = append(ListstoreItemsIntoDatabaseScrapCount, item)
			case Prefix.AddShift:
				ListstoreItemsIntoDatabaseShift = append(ListstoreItemsIntoDatabaseShift, item)
			case Prefix.UniqueProductScrap:
				ListstoreItemsIntoDatabaseUniqueProductScrap = append(ListstoreItemsIntoDatabaseUniqueProductScrap, item)
			case Prefix.AddProduct:
				ListstoreItemsIntoDatabaseAddProduct = append(ListstoreItemsIntoDatabaseAddProduct, item)
			case Prefix.AddOrder:
				ListstoreItemsIntoDatabaseAddOrder = append(ListstoreItemsIntoDatabaseAddOrder, item)
			case Prefix.StartOrder:
				ListstoreItemsIntoDatabaseStartOrder = append(ListstoreItemsIntoDatabaseStartOrder, item)
			case Prefix.EndOrder:
				ListstoreItemsIntoDatabaseEndOrder = append(ListstoreItemsIntoDatabaseEndOrder, item)
			case Prefix.AddMaintenanceActivity:
				ListstoreItemsIntoDatabaseAddMaintenanceActivity = append(ListstoreItemsIntoDatabaseAddMaintenanceActivity, item)
			case Prefix.ProductTag:
				ListstoreItemsIntoDatabaseProductTag = append(ListstoreItemsIntoDatabaseProductTag, item)
			case Prefix.ProductTagString:
				ListstoreItemsIntoDatabaseProductTagString = append(ListstoreItemsIntoDatabaseProductTagString, item)
			case Prefix.AddParentToChild:
				ListstoreItemsIntoDatabaseAddParentToChild = append(ListstoreItemsIntoDatabaseAddParentToChild, item)
			case Prefix.ModifyState:
				ListmodifyStateInDatabase = append(ListmodifyStateInDatabase, item)
			case Prefix.ModifyProducesPieces:
				ListmodifyInDatabaseModifyCountAndScrap = append(ListmodifyInDatabaseModifyCountAndScrap, item)
			case Prefix.DeleteShiftById:
				ListdeleteShiftInDatabaseById = append(ListdeleteShiftInDatabaseById, item)
			case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
				ListdeleteShiftInDatabaseByAssetIdAndTimestamp = append(ListdeleteShiftInDatabaseByAssetIdAndTimestamp, item)
			default:
				zap.S().Errorf("GQ Item with invalid Prefix! ", item.Prefix)
			}
		}

		var processError error
		var faultystoreItemsIntoDatabaseProcessValueFloat64 []QueueObject
		var faultystoreItemsIntoDatabaseProcessValue []QueueObject
		var faultystoreItemsIntoDatabaseProcessValueString []QueueObject
		var faultystoreItemsIntoDatabaseCount []QueueObject
		var faultystoreItemsIntoDatabaseRecommendation []QueueObject
		var faultystoreItemsIntoDatabaseState []QueueObject
		var faultystoreItemsIntoDatabaseUniqueProduct []QueueObject
		var faultystoreItemsIntoDatabaseScrapCount []QueueObject
		var faultystoreItemsIntoDatabaseShift []QueueObject
		var faultystoreItemsIntoDatabaseUniqueProductScrap []QueueObject
		var faultystoreItemsIntoDatabaseAddProduct []QueueObject
		var faultystoreItemsIntoDatabaseAddOrder []QueueObject
		var faultystoreItemsIntoDatabaseStartOrder []QueueObject
		var faultystoreItemsIntoDatabaseEndOrder []QueueObject
		var faultystoreItemsIntoDatabaseAddMaintenanceActivity []QueueObject
		var faultystoreItemsIntoDatabaseProductTag []QueueObject
		var faultystoreItemsIntoDatabaseProductTagString []QueueObject
		var faultystoreItemsIntoDatabaseAddParentToChild []QueueObject
		var faultymodifyStateInDatabase []QueueObject
		var faultymodifyInDatabaseModifyCountAndScrap []QueueObject
		var faultydeleteShiftInDatabaseById []QueueObject
		var faultydeleteShiftInDatabaseByAssetIdAndTimestamp []QueueObject

		faultystoreItemsIntoDatabaseProcessValueFloat64, processError = storeItemsIntoDatabaseProcessValueFloat64(ListstoreItemsIntoDatabaseProcessValueFloat64)
		faultystoreItemsIntoDatabaseProcessValue, processError = storeItemsIntoDatabaseProcessValue(ListstoreItemsIntoDatabaseProcessValue)
		faultystoreItemsIntoDatabaseProcessValueString, processError = storeItemsIntoDatabaseProcessValueString(ListstoreItemsIntoDatabaseProcessValueString)
		faultystoreItemsIntoDatabaseCount, processError = storeItemsIntoDatabaseCount(ListstoreItemsIntoDatabaseCount)
		faultystoreItemsIntoDatabaseRecommendation, processError = storeItemsIntoDatabaseRecommendation(ListstoreItemsIntoDatabaseRecommendation)
		faultystoreItemsIntoDatabaseState, processError = storeItemsIntoDatabaseState(ListstoreItemsIntoDatabaseState)
		faultystoreItemsIntoDatabaseUniqueProduct, processError = storeItemsIntoDatabaseUniqueProduct(ListstoreItemsIntoDatabaseUniqueProduct)
		faultystoreItemsIntoDatabaseScrapCount, processError = storeItemsIntoDatabaseScrapCount(ListstoreItemsIntoDatabaseScrapCount)
		faultystoreItemsIntoDatabaseShift, processError = storeItemsIntoDatabaseShift(ListstoreItemsIntoDatabaseShift)
		faultystoreItemsIntoDatabaseUniqueProductScrap, processError = storeItemsIntoDatabaseUniqueProductScrap(ListstoreItemsIntoDatabaseUniqueProductScrap)
		faultystoreItemsIntoDatabaseAddProduct, processError = storeItemsIntoDatabaseAddProduct(ListstoreItemsIntoDatabaseAddProduct)
		faultystoreItemsIntoDatabaseAddOrder, processError = storeItemsIntoDatabaseAddOrder(ListstoreItemsIntoDatabaseAddOrder)
		faultystoreItemsIntoDatabaseStartOrder, processError = storeItemsIntoDatabaseStartOrder(ListstoreItemsIntoDatabaseStartOrder)
		faultystoreItemsIntoDatabaseEndOrder, processError = storeItemsIntoDatabaseEndOrder(ListstoreItemsIntoDatabaseEndOrder)
		faultystoreItemsIntoDatabaseAddMaintenanceActivity, processError = storeItemsIntoDatabaseAddMaintenanceActivity(ListstoreItemsIntoDatabaseAddMaintenanceActivity)
		faultystoreItemsIntoDatabaseProductTag, processError = storeItemsIntoDatabaseProductTag(ListstoreItemsIntoDatabaseProductTag)
		faultystoreItemsIntoDatabaseProductTagString, processError = storeItemsIntoDatabaseProductTagString(ListstoreItemsIntoDatabaseProductTagString)
		faultystoreItemsIntoDatabaseAddParentToChild, processError = storeItemsIntoDatabaseAddParentToChild(ListstoreItemsIntoDatabaseAddParentToChild)
		faultymodifyStateInDatabase, processError = modifyStateInDatabase(ListmodifyStateInDatabase)
		faultymodifyInDatabaseModifyCountAndScrap, processError = modifyInDatabaseModifyCountAndScrap(ListmodifyInDatabaseModifyCountAndScrap)
		faultydeleteShiftInDatabaseById, processError = deleteShiftInDatabaseById(ListdeleteShiftInDatabaseById)
		faultydeleteShiftInDatabaseByAssetIdAndTimestamp, processError = deleteShiftInDatabaseByAssetIdAndTimestamp(ListdeleteShiftInDatabaseByAssetIdAndTimestamp)

		var faulty []QueueObject
		faulty = append(faulty, faultystoreItemsIntoDatabaseProcessValueFloat64...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseProcessValue...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseProcessValueString...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseCount...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseRecommendation...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseState...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseUniqueProduct...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseScrapCount...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseShift...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseUniqueProductScrap...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseAddProduct...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseAddOrder...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseStartOrder...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseEndOrder...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseAddMaintenanceActivity...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseProductTag...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseProductTagString...)
		faulty = append(faulty, faultystoreItemsIntoDatabaseAddParentToChild...)
		faulty = append(faulty, faultymodifyStateInDatabase...)
		faulty = append(faulty, faultymodifyInDatabaseModifyCountAndScrap...)
		faulty = append(faulty, faultydeleteShiftInDatabaseById...)
		faulty = append(faulty, faultydeleteShiftInDatabaseByAssetIdAndTimestamp...)

		if processError != nil {
			zap.S().Warnf("processError: %s", processError)
			for _, queueObject := range faulty {
				err := ReInsertItemOnFailure(pg, queueObject, prio)
				if err != nil {
					zap.S().Errorf("Failed to re-queue failed item, shutting down !", err)
					ShutdownApplicationGraceful()
					return
				}
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

func GetNextItemsWithPrio(pg *goque.PriorityQueue, prio uint8) (items []QueueObject, err error) {
	for i := 0; i < 1000; i++ {
		var item *goque.PriorityItem
		item, err = pg.DequeueByPriority(prio)
		if err != nil {
			if err == goque.ErrEmpty {
				break
			}
		} else {
			zap.S().Errorf("Failed to dequeue items, shutting down", err)
			ShutdownApplicationGraceful()
			return
		}
		var qitem QueueObject
		err = item.ToObjectFromJSON(&qitem)
		if err != nil {
			zap.S().Errorf("Failed to unmarshal item, shutting down", err)
			ShutdownApplicationGraceful()
			return
		}
		items = append(items, qitem)
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
