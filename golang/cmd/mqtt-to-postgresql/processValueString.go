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

func (r ValueStringHandler) Setup() (err error) {
	const queuePathDB = "/data/ValueString"
	r.pg, err = SetupQueue(queuePathDB)
	if err != nil {
		zap.S().Errorf("Error setting up remote queue (%s)", queuePathDB, err)
		return
	}
	defer CloseQueue(r.pg)
	return
}

func (r ValueStringHandler) process() {
	for !r.shutdown {
		//TODO
	}
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
