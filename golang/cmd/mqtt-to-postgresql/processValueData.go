package main

import (
	"encoding/json"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type processValueQueue struct {
	DBAssetID    uint32
	TimestampMs  uint64
	Name         string
	ValueFloat64 float64
	ValueInt32   int32
	IsFloat64    bool
}

type ValueDataHandler struct {
	pg       *goque.PriorityQueue
	shutdown bool
}

func (r ValueDataHandler) Setup() (err error) {
	const queuePathDB = "/data/ValueData"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r ValueDataHandler) process() {
	for !r.shutdown {
		//TODO
	}
}

func (r ValueDataHandler) enqueue(bytes []byte, priority uint8) {
	_, err := r.pg.Enqueue(priority, bytes)
	if err != nil {
		zap.S().Warnf("Failed to enqueue item", bytes)
		return
	}
}

func (r ValueDataHandler) Shutdown() (err error) {
	r.shutdown = true
	err = CloseQueue(r.pg)
	return
}

func (r ValueDataHandler) EnqueueMQTT(customerID string, location string, assetID string, payload []byte) {

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
					newObject := processValueQueue{
						DBAssetID:    DBassetID,
						TimestampMs:  timestampMs,
						Name:         k,
						ValueFloat64: valueFloat64,
						ValueInt32:   -1,
						IsFloat64:    true,
					}
					marshal, err := json.Marshal(newObject)
					if err != nil {
						return
					}
					r.enqueue(marshal, 0)
					break
				}
				newObject := processValueQueue{
					DBAssetID:    DBassetID,
					TimestampMs:  timestampMs,
					Name:         k,
					ValueFloat64: -1,
					ValueInt32:   value,
					IsFloat64:    false,
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
