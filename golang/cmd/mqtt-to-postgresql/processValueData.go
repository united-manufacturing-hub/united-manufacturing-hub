package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
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
	pgI32    *goque.PriorityQueue
	pgF64    *goque.PriorityQueue
	shutdown bool
}

func NewValueDataHandler() (handler *ValueDataHandler) {
	const queuePathDBI32 = "/data/ValueDataI32"
	var err error
	var pgI32 *goque.PriorityQueue
	pgI32, err = SetupQueue(queuePathDBI32)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDBI32, err)
		return
	}

	const queuePathDBF64 = "/data/ValueDataF64"
	var pgF64 *goque.PriorityQueue
	pgF64, err = SetupQueue(queuePathDBF64)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDBF64, err)
		return
	}

	handler = &ValueDataHandler{
		pgF64:    pgF64,
		pgI32:    pgI32,
		shutdown: false,
	}
	return
}

func (r ValueDataHandler) reportLength() {
	for !r.shutdown {
		time.Sleep(10 * time.Second)
		if r.pgI32.Length() > 0 {
			zap.S().Debugf("ValueDataHandler queue length (I32): %d", r.pgI32.Length())
		}
		if r.pgF64.Length() > 0 {
			zap.S().Debugf("ValueDataHandler queue length (F64): %d", r.pgF64.Length())
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
	for !r.shutdown {
		items = r.dequeueI32()
		faultyItems, err := storeItemsIntoDatabaseProcessValue(items)
		if err != nil {
			return
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
	}
}
func (r ValueDataHandler) processF64() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeueF64()
		faultyItems, err := storeItemsIntoDatabaseProcessValueFloat64(items)
		if err != nil {
			return
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
	}
}

func (r ValueDataHandler) dequeueF64() (items []*goque.PriorityItem) {
	if r.pgF64.Length() > 0 {
		item, err := r.pgF64.Dequeue()
		if err != nil {
			return
		}
		items = append(items, item)

		for true {
			nextItem, err := r.pgF64.DequeueByPriority(item.Priority)
			if err != nil {
				break
			}
			items = append(items, nextItem)
		}
	}
	return
}

func (r ValueDataHandler) dequeueI32() (items []*goque.PriorityItem) {
	if r.pgI32.Length() > 0 {
		item, err := r.pgI32.Dequeue()
		if err != nil {
			return
		}
		items = append(items, item)

		for true {
			nextItem, err := r.pgI32.DequeueByPriority(item.Priority)
			if err != nil {
				break
			}
			items = append(items, nextItem)
		}
	}
	return
}

func (r ValueDataHandler) enqueueI32(bytes []byte, priority uint8) {
	_, err := r.pgI32.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}
func (r ValueDataHandler) enqueueF64(bytes []byte, priority uint8) {
	_, err := r.pgF64.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes, err)
		return
	}
}

func (r ValueDataHandler) Shutdown() (err error) {
	r.shutdown = true
	time.Sleep(1 * time.Second)
	err = CloseQueue(r.pgI32)
	return
}

func (r ValueDataHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	zap.S().Debugf("[ValueDataHandler]")
	var parsedPayload interface{}

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return
	}

	DBassetID := GetAssetID(customerID, location, assetID)

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
