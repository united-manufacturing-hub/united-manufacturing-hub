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

// processDBQueue get's item from queue and processes it, depending on prefix
func processDBQueue(pg *goque.PriorityQueue) {

	zap.S().Infof("Starting new processDBQueue worker")
	for !shuttingDown {
		if pg.Length() == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		zap.S().Debugf("pg.Length() != 0")

		item, prio, err := GetNextItemInQueue(pg)
		if err != nil {
			if err != goque.ErrEmpty {
				err = ReInsertItemOnFailure(pg, item, prio)
				if err != nil {
					zap.S().Errorf("Failed to re-queue failed item, shutting down !", err)
					ShutdownApplicationGraceful()
					return
				}
			} else {
				err = nil
			}
		}
		if len(item.Prefix) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		zap.S().Debugf("First item: %s", item.Prefix, item.Payload)

		var items []QueueObject
		items, err = GetNextItemsWithPrio(pg, prio)
		if err != nil {
			err = ReInsertItemOnFailure(pg, item, prio)
			if err != nil {
				zap.S().Errorf("Failed to re-queue failed item, shutting down !", err)
				ShutdownApplicationGraceful()
				return
			}
		}

		items = append(items, item)
		zap.S().Debugf("Working with %d items", len(items))

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

		for _, xitem := range items {
			zap.S().Debugf("Switching: %s", xitem.Prefix)
			switch item.Prefix {
			case Prefix.ProcessValueFloat64:
				ListstoreItemsIntoDatabaseProcessValueFloat64 = append(ListstoreItemsIntoDatabaseProcessValueFloat64, xitem)
			case Prefix.ProcessValue:
				ListstoreItemsIntoDatabaseProcessValue = append(ListstoreItemsIntoDatabaseProcessValue, xitem)
			case Prefix.ProcessValueString:
				ListstoreItemsIntoDatabaseProcessValueString = append(ListstoreItemsIntoDatabaseProcessValueString, xitem)
			case Prefix.Count:
				zap.S().Debugf("Switched into Count")
				ListstoreItemsIntoDatabaseCount = append(ListstoreItemsIntoDatabaseCount, xitem)
			case Prefix.Recommendation:
				ListstoreItemsIntoDatabaseRecommendation = append(ListstoreItemsIntoDatabaseRecommendation, xitem)
			case Prefix.State:
				ListstoreItemsIntoDatabaseState = append(ListstoreItemsIntoDatabaseState, xitem)
			case Prefix.UniqueProduct:
				ListstoreItemsIntoDatabaseUniqueProduct = append(ListstoreItemsIntoDatabaseUniqueProduct, xitem)
			case Prefix.ScrapCount:
				ListstoreItemsIntoDatabaseScrapCount = append(ListstoreItemsIntoDatabaseScrapCount, xitem)
			case Prefix.AddShift:
				ListstoreItemsIntoDatabaseShift = append(ListstoreItemsIntoDatabaseShift, xitem)
			case Prefix.UniqueProductScrap:
				ListstoreItemsIntoDatabaseUniqueProductScrap = append(ListstoreItemsIntoDatabaseUniqueProductScrap, xitem)
			case Prefix.AddProduct:
				ListstoreItemsIntoDatabaseAddProduct = append(ListstoreItemsIntoDatabaseAddProduct, xitem)
			case Prefix.AddOrder:
				ListstoreItemsIntoDatabaseAddOrder = append(ListstoreItemsIntoDatabaseAddOrder, xitem)
			case Prefix.StartOrder:
				ListstoreItemsIntoDatabaseStartOrder = append(ListstoreItemsIntoDatabaseStartOrder, xitem)
			case Prefix.EndOrder:
				ListstoreItemsIntoDatabaseEndOrder = append(ListstoreItemsIntoDatabaseEndOrder, xitem)
			case Prefix.AddMaintenanceActivity:
				ListstoreItemsIntoDatabaseAddMaintenanceActivity = append(ListstoreItemsIntoDatabaseAddMaintenanceActivity, xitem)
			case Prefix.ProductTag:
				ListstoreItemsIntoDatabaseProductTag = append(ListstoreItemsIntoDatabaseProductTag, xitem)
			case Prefix.ProductTagString:
				ListstoreItemsIntoDatabaseProductTagString = append(ListstoreItemsIntoDatabaseProductTagString, xitem)
			case Prefix.AddParentToChild:
				ListstoreItemsIntoDatabaseAddParentToChild = append(ListstoreItemsIntoDatabaseAddParentToChild, xitem)
			case Prefix.ModifyState:
				ListmodifyStateInDatabase = append(ListmodifyStateInDatabase, xitem)
			case Prefix.ModifyProducesPieces:
				ListmodifyInDatabaseModifyCountAndScrap = append(ListmodifyInDatabaseModifyCountAndScrap, xitem)
			case Prefix.DeleteShiftById:
				ListdeleteShiftInDatabaseById = append(ListdeleteShiftInDatabaseById, xitem)
			case Prefix.DeleteShiftByAssetIdAndBeginTimestamp:
				ListdeleteShiftInDatabaseByAssetIdAndTimestamp = append(ListdeleteShiftInDatabaseByAssetIdAndTimestamp, xitem)
			case Prefix.RawMQTTRequeue:
				RetryMQTT(xitem, prio)
			default:
				zap.S().Errorf("GQ Item with invalid Prefix! %s, len(%d)", xitem.Prefix, len(xitem.Prefix), xitem.Payload)
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

		if len(ListstoreItemsIntoDatabaseProcessValueFloat64) > 0 {
			faultystoreItemsIntoDatabaseProcessValueFloat64, processError = storeItemsIntoDatabaseProcessValueFloat64(ListstoreItemsIntoDatabaseProcessValueFloat64)
		}
		if len(ListstoreItemsIntoDatabaseProcessValue) > 0 {
			faultystoreItemsIntoDatabaseProcessValue, processError = storeItemsIntoDatabaseProcessValue(ListstoreItemsIntoDatabaseProcessValue)
		}
		if len(ListstoreItemsIntoDatabaseProcessValueString) > 0 {
			faultystoreItemsIntoDatabaseProcessValueString, processError = storeItemsIntoDatabaseProcessValueString(ListstoreItemsIntoDatabaseProcessValueString)
		}
		if len(ListstoreItemsIntoDatabaseCount) > 0 {
			faultystoreItemsIntoDatabaseCount, processError = storeItemsIntoDatabaseCount(ListstoreItemsIntoDatabaseCount)
		}
		if len(ListstoreItemsIntoDatabaseRecommendation) > 0 {
			faultystoreItemsIntoDatabaseRecommendation, processError = storeItemsIntoDatabaseRecommendation(ListstoreItemsIntoDatabaseRecommendation)
		}
		if len(ListstoreItemsIntoDatabaseState) > 0 {
			faultystoreItemsIntoDatabaseState, processError = storeItemsIntoDatabaseState(ListstoreItemsIntoDatabaseState)
		}
		if len(ListstoreItemsIntoDatabaseUniqueProduct) > 0 {
			faultystoreItemsIntoDatabaseUniqueProduct, processError = storeItemsIntoDatabaseUniqueProduct(ListstoreItemsIntoDatabaseUniqueProduct)
		}
		if len(ListstoreItemsIntoDatabaseScrapCount) > 0 {
			faultystoreItemsIntoDatabaseScrapCount, processError = storeItemsIntoDatabaseScrapCount(ListstoreItemsIntoDatabaseScrapCount)
		}
		if len(ListstoreItemsIntoDatabaseShift) > 0 {
			faultystoreItemsIntoDatabaseShift, processError = storeItemsIntoDatabaseShift(ListstoreItemsIntoDatabaseShift)
		}
		if len(ListstoreItemsIntoDatabaseUniqueProductScrap) > 0 {
			faultystoreItemsIntoDatabaseUniqueProductScrap, processError = storeItemsIntoDatabaseUniqueProductScrap(ListstoreItemsIntoDatabaseUniqueProductScrap)
		}
		if len(ListstoreItemsIntoDatabaseAddProduct) > 0 {
			faultystoreItemsIntoDatabaseAddProduct, processError = storeItemsIntoDatabaseAddProduct(ListstoreItemsIntoDatabaseAddProduct)
		}
		if len(ListstoreItemsIntoDatabaseAddOrder) > 0 {
			faultystoreItemsIntoDatabaseAddOrder, processError = storeItemsIntoDatabaseAddOrder(ListstoreItemsIntoDatabaseAddOrder)
		}
		if len(ListstoreItemsIntoDatabaseStartOrder) > 0 {
			faultystoreItemsIntoDatabaseStartOrder, processError = storeItemsIntoDatabaseStartOrder(ListstoreItemsIntoDatabaseStartOrder)
		}
		if len(ListstoreItemsIntoDatabaseEndOrder) > 0 {
			faultystoreItemsIntoDatabaseEndOrder, processError = storeItemsIntoDatabaseEndOrder(ListstoreItemsIntoDatabaseEndOrder)
		}
		if len(ListstoreItemsIntoDatabaseAddMaintenanceActivity) > 0 {
			faultystoreItemsIntoDatabaseAddMaintenanceActivity, processError = storeItemsIntoDatabaseAddMaintenanceActivity(ListstoreItemsIntoDatabaseAddMaintenanceActivity)
		}
		if len(ListstoreItemsIntoDatabaseProductTag) > 0 {
			faultystoreItemsIntoDatabaseProductTag, processError = storeItemsIntoDatabaseProductTag(ListstoreItemsIntoDatabaseProductTag)
		}
		if len(ListstoreItemsIntoDatabaseProductTagString) > 0 {
			faultystoreItemsIntoDatabaseProductTagString, processError = storeItemsIntoDatabaseProductTagString(ListstoreItemsIntoDatabaseProductTagString)
		}
		if len(ListstoreItemsIntoDatabaseAddParentToChild) > 0 {
			faultystoreItemsIntoDatabaseAddParentToChild, processError = storeItemsIntoDatabaseAddParentToChild(ListstoreItemsIntoDatabaseAddParentToChild)
		}
		if len(ListmodifyStateInDatabase) > 0 {
			faultymodifyStateInDatabase, processError = modifyStateInDatabase(ListmodifyStateInDatabase)
		}
		if len(ListmodifyInDatabaseModifyCountAndScrap) > 0 {
			faultymodifyInDatabaseModifyCountAndScrap, processError = modifyInDatabaseModifyCountAndScrap(ListmodifyInDatabaseModifyCountAndScrap)
		}
		if len(ListdeleteShiftInDatabaseById) > 0 {
			faultydeleteShiftInDatabaseById, processError = deleteShiftInDatabaseById(ListdeleteShiftInDatabaseById)
		}
		if len(ListdeleteShiftInDatabaseByAssetIdAndTimestamp) > 0 {
			faultydeleteShiftInDatabaseByAssetIdAndTimestamp, processError = deleteShiftInDatabaseByAssetIdAndTimestamp(ListdeleteShiftInDatabaseByAssetIdAndTimestamp)
		}

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
	zap.S().Infof("processDBQueue worker shutting down")
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
	prio = priorityItem.Priority
	err = priorityItem.ToObjectFromJSON(&item)
	if err != nil {
		return QueueObject{}, 0, err
	}
	return
}

func GetNextItemsWithPrio(pg *goque.PriorityQueue, prio uint8) (items []QueueObject, err error) {
	items = make([]QueueObject, pg.Length())
	for i := 0; i < 1000; i++ {
		var item *goque.PriorityItem
		item, err = pg.DequeueByPriority(prio)
		if err != nil {
			if err == goque.ErrEmpty {
				err = nil
				break
			} else {
				zap.S().Errorf("Failed to dequeue items, shutting down", err)
				ShutdownApplicationGraceful()
				return
			}
		}
		var qitem QueueObject
		err = item.ToObjectFromJSON(&qitem)
		if err != nil {
			zap.S().Errorf("Failed to unmarshal item, shutting down", err)
			ShutdownApplicationGraceful()
			return
		}
		if len(qitem.Prefix) > 0 {
			items = append(items, qitem)
		}
	}
	return
}

// addItemWithPriorityToQueue adds an item with given priority to queue
func addItemWithPriorityToQueue(pq *goque.PriorityQueue, item QueueObject, priority uint8) (err error) {
	_, err = pq.EnqueueObjectAsJSON(priority, item)
	return
}

// addNewItemToQueue adds an item with 0 priority to queue
func addNewItemToQueue(pq *goque.PriorityQueue, payloadType string, payload []byte) (err error) {
	item := QueueObject{
		Prefix:  payloadType,
		Payload: payload,
	}
	err = addItemWithPriorityToQueue(pq, item, 0)
	return
}

// addItemWithPriorityToQueue adds an item with given priority to queue
func addRawItemWithPriorityToQueue(pq *goque.PriorityQueue, payloadType string, payload []byte, priority uint8) (err error) {
	item := QueueObject{
		Prefix:  payloadType,
		Payload: payload,
	}
	err = addItemWithPriorityToQueue(pq, item, priority)
	return
}

// reportQueueLength prints the current Queue length
func reportQueueLength(pg *goque.PriorityQueue) {
	for true {
		zap.S().Infof("Current elements in queue: %d", pg.Length())
		time.Sleep(10 * time.Second)
	}
}
