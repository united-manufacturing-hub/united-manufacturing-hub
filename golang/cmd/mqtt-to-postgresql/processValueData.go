package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

type processValueQueueI32 struct {
	DBAssetID   uint32
	TimestampMs uint64
	Name        string
	ValueInt32  int32
}
type processValueQueueF64 struct {
	DBAssetID    uint32
	TimestampMs  uint64
	Name         string
	ValueFloat64 float64
}

type ValueDataHandler struct {
	priorityQueueI32 *goque.PriorityQueue
	priorityQueueF64 *goque.PriorityQueue
	shutdown         bool
}

func NewValueDataHandler() (handler *ValueDataHandler) {
	const queuePathDBI32 = "/data/ValueDataI32"
	var err error
	var priorityQueueI32 *goque.PriorityQueue
	priorityQueueI32, err = SetupQueue(queuePathDBI32)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDBI32, err)
		return
	}

	const queuePathDBF64 = "/data/ValueDataF64"
	var priorityQueueF64 *goque.PriorityQueue
	priorityQueueF64, err = SetupQueue(queuePathDBF64)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDBF64, err)
		return
	}

	handler = &ValueDataHandler{
		priorityQueueF64: priorityQueueF64,
		priorityQueueI32: priorityQueueI32,
		shutdown:         false,
	}
	return
}

func (r ValueDataHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.priorityQueueI32.Length() > 0 {
			zap.S().Debugf("ValueDataHandler queue length (I32): %d", r.priorityQueueI32.Length())
		}
		if r.priorityQueueF64.Length() > 0 {
			zap.S().Debugf("ValueDataHandler queue length (F64): %d", r.priorityQueueF64.Length())
		}
	}
}
func (r ValueDataHandler) Setup() {
	go r.reportLength()
	go r.processI32()
	go r.processF64()
}

func (r ValueDataHandler) processI32() {
	var items []*goque.PriorityItem
	loopsWithError := int64(0)
	for !r.shutdown {
		items = r.dequeueI32()
		if len(items) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		faultyItems, err := storeItemsIntoDatabaseProcessValue(items)

		if err != nil {
			zap.S().Errorf("err: %s", err)
			switch GetPostgresErrorRecoveryOptions(err) {
			case Unrecoverable:
				ShutdownApplicationGraceful()
			}
		}
		// Empty the array, without de-allocating memory
		items = items[:0]
		for _, faultyItem := range faultyItems {
			var prio uint8
			prio = faultyItem.Priority + 1
			if faultyItem.Priority >= 255 {
				prio = 254
			}
			r.enqueueI32(faultyItem.Value, prio)
		}

		if err != nil || len(faultyItems) > 0 {
			loopsWithError += 1
		} else {
			loopsWithError = 0
		}

		internal.SleepBackedOff(loopsWithError, 10000*time.Nanosecond, 1000*time.Millisecond)
	}
}

func (r ValueDataHandler) processF64() {
	var items []*goque.PriorityItem
	loopsWithError := int64(0)
	for !r.shutdown {
		items = r.dequeueF64()
		if len(items) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		faultyItems, err := storeItemsIntoDatabaseProcessValueFloat64(items)

		if err != nil {
			zap.S().Errorf("err: %s", err)
			switch GetPostgresErrorRecoveryOptions(err) {
			case Unrecoverable:
				ShutdownApplicationGraceful()
			}
		}
		// Empty the array, without de-allocating memory
		items = items[:0]
		for _, faultyItem := range faultyItems {
			var prio uint8
			prio = faultyItem.Priority + 1
			if faultyItem.Priority >= 255 {
				prio = 254
			}
			r.enqueueF64(faultyItem.Value, prio)
		}

		if err != nil || len(faultyItems) > 0 {
			loopsWithError += 1
		} else {
			loopsWithError = 0
		}

		internal.SleepBackedOff(loopsWithError, 10000*time.Nanosecond, 1000*time.Millisecond)
	}
}

func (r ValueDataHandler) dequeueF64() (items []*goque.PriorityItem) {
	if r.priorityQueueF64.Length() > 0 {
		item, err := r.priorityQueueF64.Dequeue()
		if err != nil {
			return
		}
		items = append(items, item)

		for true {
			nextItem, err := r.priorityQueueF64.DequeueByPriority(item.Priority)
			if err != nil {
				break
			}
			items = append(items, nextItem)
		}
	}
	return
}

func (r ValueDataHandler) dequeueI32() (items []*goque.PriorityItem) {
	if r.priorityQueueI32.Length() > 0 {
		item, err := r.priorityQueueI32.Dequeue()
		if err != nil {
			return
		}
		items = append(items, item)

		for true {
			nextItem, err := r.priorityQueueI32.DequeueByPriority(item.Priority)
			if err != nil {
				break
			}
			items = append(items, nextItem)
		}
	}
	return
}

func (r ValueDataHandler) enqueueI32(bytes []byte, priority uint8) {
	_, err := r.priorityQueueI32.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}
func (r ValueDataHandler) enqueueF64(bytes []byte, priority uint8) {
	_, err := r.priorityQueueF64.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r ValueDataHandler) Shutdown() (err error) {
	zap.S().Warnf("[ValueDataHandler] shutting down, Queue length: F64: %d,I32: %d", r.priorityQueueF64.Length(), r.priorityQueueI32.Length())
	r.shutdown = true

	err = CloseQueue(r.priorityQueueI32)
	return
}

func (r ValueDataHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte, recursionDepth int64) {
	zap.S().Debugf("[ValueDataHandler]")
	var parsedPayload interface{}

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID, success := GetAssetID(customerID, location, assetID, 0)
	if !success {
		go func() {
			if r.shutdown {
				storedRawMQTTHandler.EnqueueMQTT(customerID, location, assetID, payload, Prefix.AddOrder, recursionDepth+1)
			} else {
				internal.SleepBackedOff(recursionDepth, 10000*time.Nanosecond, 1000*time.Millisecond)
				r.EnqueueMQTT(customerID, location, assetID, payload, recursionDepth+1)
			}
		}()
		return
	}

	// process unknown data structure according to https://blog.golang.org/json
	m := parsedPayload.(map[string]interface{})

	if val, ok := m["timestamp_ms"]; ok { //if timestamp_ms key exists (https://stackoverflow.com/questions/2050391/how-to-check-if-a-map-contains-a-key-in-go)
		timestampMs, ok := val.(uint64)
		if !ok {
			timestampMsFloat, ok2 := val.(float64)
			if !ok2 {
				zap.S().Errorf("Timestamp not int64 nor float64", payload, val)
				return
			}
			if timestampMsFloat < 0 {
				zap.S().Errorf("Timestamp is negative !", payload, val)
				return
			}
			timestampMs = uint64(timestampMsFloat)
		}

		// loop through map
		for k, v := range m {
			switch k {
			case "timestamp_ms":
			case "measurement":
			case "serial_number":
				break //ignore them
			default:
				value, ok := v.(int32)
				if !ok {
					valueFloat64, ok2 := v.(float64)
					if !ok2 {
						zap.S().Errorf("Process value recieved that is not an integer nor float", k, v)
						break
					}
					newObject := processValueQueueF64{
						DBAssetID:    DBassetID,
						TimestampMs:  timestampMs,
						Name:         k,
						ValueFloat64: valueFloat64,
					}
					marshal, err := json.Marshal(newObject)
					if err != nil {
						return
					}
					r.enqueueF64(marshal, 0)
					break
				}
				newObject := processValueQueueI32{
					DBAssetID:   DBassetID,
					TimestampMs: timestampMs,
					Name:        k,
					ValueInt32:  value,
				}
				marshal, err := json.Marshal(newObject)
				if err != nil {
					return
				}
				r.enqueueI32(marshal, 0)
			}
		}

	}
	return
}
