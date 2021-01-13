package main

import (
	// postgreSQL integration

	"database/sql"
	"encoding/json"

	"go.uber.org/zap"
)

type count struct {
	Count       int   `json:"count"`
	TimestampMs int64 `json:"timestamp_ms"`
}

type addShift struct {
	TimestampMs    int64 `json:"timestamp_ms"`
	TimestampMsEnd int64 `json:"timestamp_ms_end"`
}

/*
uid TEXT NOT NULL,
asset_id            SERIAL REFERENCES assetTable (id),
begin_timestamp_ms                TIMESTAMPTZ NOT NULL,
end_timestamp_ms                TIMESTAMPTZ                         NOT NULL,
product_id TEXT NOT NULL,
is_scrap BOOLEAN NOT NULL,
quality_class TEXT NOT NULL,
station_id TEXT NOT NULL,
*/
type uniqueProduct struct {
	UID              string `json:"UID"`
	TimestampMsBegin int64  `json:"begin_timestamp_ms"`
	TimestampMsEnd   int64  `json:"end_timestamp_ms"`
	ProductID        string `json:"productID"`
	IsScrap          bool   `json:"isScrap"`
	QualityClass     string `json:"qualityClass"`
	StationID        string `json:"stationID"`
}

/*
product_id SERIAL PRIMARY KEY,
    product_name VARCHAR(40) NOT NULL,
	asset_id SERIAL REFERENCES assetTable (id),
    time_per_unit_in_seconds REAL NOT NULL,
	UNIQUE(product_name, asset_id),
	CHECK (time_per_unit_in_seconds > 0)
*/
type addProduct struct {
	ProductName          string  `json:"product_id"`
	TimePerUnitInSeconds float64 `json:"time_per_unit_in_seconds"`
}

/*
order_id SERIAL PRIMARY KEY,
    order_name VARCHAR(40) NOT NULL,
    product_id SERIAL REFERENCES productTable (product_id),
    begin_timestamp TIMESTAMPZ,
    end_timestamp TIMESTAMPZ,
    target_units INTEGER,
    asset_id SERIAL REFERENCES assetTable (id),
    unique (asset_id, order_id),
    CHECK (begin_timestamp < end_timestamp),
	CHECK (target_units > 0)
*/
type addOrder struct {
	ProductName string `json:"product_id"`
	OrderName   string `json:"order_id"`
	TargetUnits int    `json:"target_units"`
}

type startOrder struct {
	TimestampMs int64  `json:"timestamp_ms"`
	OrderName   string `json:"order_id"`
}

type endOrder struct {
	TimestampMs int64  `json:"timestamp_ms"`
	OrderName   string `json:"order_id"`
}

type addMaintenanceActivity struct {
	TimestampMs   int64  `json:"timestamp_ms"`
	ComponentName string `json:"component"`
	Activity      int    `json:"activity"`
}

type state struct {
	State       int   `json:"state"`
	TimestampMs int64 `json:"timestamp_ms"`
}

type recommendationStruct struct {
	UID                  string
	TimestampMs          int64 `json:"timestamp_ms"`
	Customer             string
	Location             string
	Asset                string
	RecommendationType   int
	Enabled              bool
	RecommendationValues string
	DiagnoseTextDE       string
	DiagnoseTextEN       string
	RecommendationTextDE string
	RecommendationTextEN string
}

// ProcessStateData processes an incoming state message
func ProcessStateData(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload state

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	state := parsedPayload.State
	timestampMs := parsedPayload.TimestampMs

	StoreIntoStateTable(timestampMs, DBassetID, state)
}

// ProcessCountData processes an incoming count message
func ProcessCountData(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload count

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	count := parsedPayload.Count
	timestampMs := parsedPayload.TimestampMs

	StoreIntoCountTable(timestampMs, DBassetID, count)
}

// ProcessAddShift adds a new shift to the database
func ProcessAddShift(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload addShift

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	timestampMs := parsedPayload.TimestampMs
	timestampMsEnd := parsedPayload.TimestampMsEnd

	StoreIntoShiftTable(timestampMs, DBassetID, timestampMsEnd)
}

// ProcessAddMaintenanceActivity adds a new maintenance activity to the database
func ProcessAddMaintenanceActivity(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload addMaintenanceActivity

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	timestampMs := parsedPayload.TimestampMs
	componentName := parsedPayload.ComponentName

	componentID := GetComponentID(DBassetID, componentName)
	if componentID == 0 {
		zap.S().Errorf("GetComponentID failed")
		return
	}
	activityType := parsedPayload.Activity

	StoreIntoMaintenancewActivitiesTable(timestampMs, componentID, activityType)
}

// ProcessUniqueProduct adds a new uniqueProduct to the database
func ProcessUniqueProduct(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload uniqueProduct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	UID := parsedPayload.UID

	timestampMsBegin := parsedPayload.TimestampMsBegin
	timestampMsEnd := parsedPayload.TimestampMsEnd
	productID := parsedPayload.ProductID
	isScrap := parsedPayload.IsScrap
	qualityClass := ""
	stationID := parsedPayload.StationID

	StoreIntoUniqueProductTable(UID, DBassetID, timestampMsBegin, timestampMsEnd, productID, isScrap, qualityClass, stationID)
}

// ProcessAddProduct adds a new product to the database
func ProcessAddProduct(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload addProduct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	productName := parsedPayload.ProductName
	timePerUnitInSeconds := parsedPayload.TimePerUnitInSeconds

	StoreIntoProductTable(DBassetID, productName, timePerUnitInSeconds)
}

// ProcessAddOrder adds a new order without begin and end timestamp to the database
func ProcessAddOrder(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload addOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	productName := parsedPayload.ProductName
	orderName := parsedPayload.OrderName
	targetUnits := parsedPayload.TargetUnits

	productID, err := GetProductID(DBassetID, productName)
	if err == sql.ErrNoRows {
		zap.S().Errorf("Product does not exist yet", DBassetID, productName, orderName)
		return
	} else if err != nil { // never executed
		PQErrorHandling("GetProductID db.QueryRow()", err)
	}

	StoreIntoOrderTable(orderName, productID, targetUnits, DBassetID)
}

// ProcessStartOrder starts an order by adding beginTimestamp
func ProcessStartOrder(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload startOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	timestampMs := parsedPayload.TimestampMs
	orderName := parsedPayload.OrderName

	UpdateBeginTimestampInOrderTable(orderName, timestampMs, DBassetID)
}

// ProcessEndOrder starts an order by adding endTimestamp
func ProcessEndOrder(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload endOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	timestampMs := parsedPayload.TimestampMs
	orderName := parsedPayload.OrderName

	UpdateEndTimestampInOrderTable(orderName, timestampMs, DBassetID)
}

// ProcessRecommendationData processes an incoming count message
func ProcessRecommendationData(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload recommendationStruct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	uid := parsedPayload.UID
	timestampMs := parsedPayload.TimestampMs
	recommendationType := parsedPayload.RecommendationType
	recommendationValues := parsedPayload.RecommendationValues
	recommendationDiagnoseTextDE := parsedPayload.DiagnoseTextDE
	recommendationDiagnoseTextEN := parsedPayload.DiagnoseTextEN
	recommendationTextDE := parsedPayload.RecommendationTextDE
	recommendationTextEN := parsedPayload.RecommendationTextEN
	enabled := parsedPayload.Enabled

	StoreRecommendation(timestampMs, uid, recommendationType, enabled, recommendationValues, recommendationTextEN, recommendationTextDE, recommendationDiagnoseTextDE, recommendationDiagnoseTextEN)
}

// ProcessProcessValueData processes an incoming processValue message
func ProcessProcessValueData(customerID string, location string, assetID string, payloadType string, payload []byte) {
	var parsedPayload interface{}

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	// process unknown data structure according to https://blog.golang.org/json
	m := parsedPayload.(map[string]interface{})

	if val, ok := m["timestamp_ms"]; ok { //if timestamp_ms key exists (https://stackoverflow.com/questions/2050391/how-to-check-if-a-map-contains-a-key-in-go)
		timestampMs, ok := val.(int64)
		if !ok {
			timestampMsFloat, ok2 := val.(float64)
			if !ok2 {
				zap.S().Errorf("Timestamp not int64 nor float64", payload, val)
				return
			}
			timestampMs = int64(timestampMsFloat)
		}

		// loop through map
		for k, v := range m {
			switch k {
			case "timestamp_ms":
			case "measurement":
			case "serial_number":
				break //ignore them
			default:
				value, ok := v.(int)
				if !ok {
					valueFloat64, ok2 := v.(float64)
					if !ok2 {
						zap.S().Errorf("Process value recieved that is not an integer nor float", k, v)
						break
					}
					StoreIntoprocessValueTableFloat64(timestampMs, DBassetID, valueFloat64, k)
					break
				}
				StoreIntoprocessValueTable(timestampMs, DBassetID, value, k)
			}
		}

	}
}
