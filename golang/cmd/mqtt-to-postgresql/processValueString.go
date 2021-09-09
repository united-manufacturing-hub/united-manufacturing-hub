package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type processValueStringQueue struct {
	DBAssetID   uint32
	TimestampMs uint64
	Name        string
	Value       string
}

type ValueStringHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func NewValueStringHandler() (handler *ValueStringHandler) {
	const queuePathDB = "/data/ValueString"
	var pg *goque.PriorityQueue
	var err error
	pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(pg)
	handler = &ValueStringHandler{
		pg:       pg,
		shutdown: false,
	}
	return
}

func (r ValueStringHandler) process() {
	var items []*goque.PriorityItem
	for !r.shutdown {
		items = r.dequeue()
		faultyItems, err := storeItemsIntoDatabaseProcessValueString(items)
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
			r.enqueue(faultyItem.Value, prio)
		}
	}
}

func (r ValueStringHandler) dequeue() (items []*goque.PriorityItem) {
	if r.pg.Length() > 0 {
		item, err := r.pg.Dequeue()
		if err != nil {
			return
		}
		items = append(items, item)

		for true {
			nextItem, err := r.pg.DequeueByPriority(item.Priority)
			if err != nil {
				break
			}
			items = append(items, nextItem)
		}
	}
	return
}

func (r ValueStringHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r ValueStringHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r ValueStringHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {
	zap.S().Debugf("[ValueStringHandler]")
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
			case "measurement": //only to ignore legacy messages todo: highlight in documentation
			case "serial_number": //only to ignore legacy messages todo: highlight in documentation
				break //ignore them
			default:
				value, ok := v.(string)
				if !ok {
					zap.S().Errorf("Process value recieved that is not a string", k, v)
					break
				}
				newObject := processValueStringQueue{
					DBAssetID:   DBassetID,
					TimestampMs: timestampMs,
					Name:        k,
					Value:       value,
				}
				marshal, err := json.Marshal(newObject)
				if err != nil {
					return
				}
				r.enqueue(marshal, 0)
			}
		}

	}
	return
}
