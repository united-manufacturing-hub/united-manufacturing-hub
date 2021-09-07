package main

import (
	// postgreSQL integration

	"database/sql"
	"encoding/json"
	"errors"
	"github.com/beeker1121/goque"
	"go.uber.org/zap"
)

type stateQueue struct {
	DBAssetID   uint32
	State       uint32
	TimestampMs uint32
}
type state struct {
	State       uint32 `json:"state"`
	TimestampMs uint32 `json:"timestamp_ms"`
}

// ProcessStateData processes an incoming state message
func ProcessStateData(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload state

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	newObject := stateQueue{
		TimestampMs: parsedPayload.TimestampMs,
		State:       parsedPayload.State,
		DBAssetID:   DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.State, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}

	return nil
}

type countQueue struct {
	DBAssetID   uint32
	Count       uint32
	Scrap       uint32
	TimestampMs uint32
}
type count struct {
	Count       uint32 `json:"count"`
	Scrap       uint32 `json:"scrap"`
	TimestampMs uint32 `json:"timestamp_ms"`
}

// ProcessCountData processes an incoming count message
func ProcessCountData(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload count

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	// this should not happen. Throw a warning message and ignore (= do not try to store in database)
	if parsedPayload.Count <= 0 {
		zap.S().Warnf("count <= 0", customerID, location, assetID, payload, parsedPayload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	newObject := countQueue{
		TimestampMs: parsedPayload.TimestampMs,
		Count:       parsedPayload.Count,
		Scrap:       parsedPayload.Scrap,
		DBAssetID:   DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.Count, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type scrapCountQueue struct {
	DBAssetID   uint32
	Scrap       uint32
	TimestampMs uint32
}
type scrapCount struct {
	Scrap       uint32 `json:"scrap"`
	TimestampMs uint32 `json:"timestamp_ms"`
}

// ProcessScrapCountData processes an incoming scrapCount message
func ProcessScrapCountData(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload scrapCount

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	newObject := scrapCountQueue{
		TimestampMs: parsedPayload.TimestampMs,
		Scrap:       parsedPayload.Scrap,
		DBAssetID:   DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.ScrapCount, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type addShiftQueue struct {
	DBAssetID      uint32
	TimestampMs    uint32
	TimestampMsEnd uint32
}
type addShift struct {
	TimestampMs    uint32 `json:"timestamp_ms"`
	TimestampMsEnd uint32 `json:"timestamp_ms_end"`
}

// ProcessAddShift adds a new shift to the database
func ProcessAddShift(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload addShift

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := addShiftQueue{
		TimestampMs:    parsedPayload.TimestampMs,
		TimestampMsEnd: parsedPayload.TimestampMsEnd,
		DBAssetID:      DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.AddShift, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type addMaintenanceActivityQueue struct {
	DBAssetID     uint32
	TimestampMs   uint32
	ComponentName string
	Activity      int32
	ComponentID   int32
}
type addMaintenanceActivity struct {
	TimestampMs   uint32 `json:"timestamp_ms"`
	ComponentName string `json:"component"`
	Activity      int32  `json:"activity"`
}

// ProcessAddMaintenanceActivity adds a new maintenance activity to the database
func ProcessAddMaintenanceActivity(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload addMaintenanceActivity

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	componentID := GetComponentID(DBassetID, parsedPayload.ComponentName)
	if componentID == 0 {
		zap.S().Errorf("GetComponentID failed")
		return nil
	}

	newObject := addMaintenanceActivityQueue{
		DBAssetID:     DBassetID,
		TimestampMs:   parsedPayload.TimestampMs,
		ComponentName: parsedPayload.ComponentName,
		ComponentID:   componentID,
		Activity:      parsedPayload.Activity,
	}
	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.AddMaintenanceActivity, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type uniqueProductQueue struct {
	DBAssetID                  uint32
	BeginTimestampMs           uint32 `json:"begin_timestamp_ms"`
	EndTimestampMs             uint32 `json:"end_timestamp_ms"`
	ProductID                  int32  `json:"productID"`
	IsScrap                    bool   `json:"isScrap"`
	UniqueProductAlternativeID string `json:"uniqueProductAlternativeID"`
}
type uniqueProduct struct {
	BeginTimestampMs           uint32 `json:"begin_timestamp_ms"`
	EndTimestampMs             uint32 `json:"end_timestamp_ms"`
	ProductName                string `json:"productID"`
	IsScrap                    bool   `json:"isScrap"`
	UniqueProductAlternativeID string `json:"uniqueProductAlternativeID"`
}

var ErrTryLater = errors.New("MQTT message could not be processed, please try later")

// ProcessUniqueProduct adds a new uniqueProduct to the database
func ProcessUniqueProduct(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload uniqueProduct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	productID, err := GetProductID(DBassetID, parsedPayload.ProductName)
	if err == sql.ErrNoRows {
		zap.S().Errorf("Product does not exist yet", DBassetID, parsedPayload.ProductName)
		return ErrTryLater
	} else if err != nil { // never executed
		PQErrorHandling("GetProductID db.QueryRow()", err)
	}

	newObject := uniqueProductQueue{
		DBAssetID:                  DBassetID,
		BeginTimestampMs:           parsedPayload.BeginTimestampMs,
		EndTimestampMs:             parsedPayload.EndTimestampMs,
		ProductID:                  productID,
		IsScrap:                    parsedPayload.IsScrap,
		UniqueProductAlternativeID: parsedPayload.UniqueProductAlternativeID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.UniqueProduct, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type scrapUniqueProductQueue struct {
	DBAssetID uint32
	UID       string
}
type scrapUniqueProduct struct {
	UID string `json:"UID"`
}

// ProcessScrapUniqueProduct sets isScrap of a uniqueProduct to true
func ProcessScrapUniqueProduct(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload scrapUniqueProduct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := scrapUniqueProductQueue{
		UID:       parsedPayload.UID,
		DBAssetID: DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.UniqueProductScrap, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type addProductQueue struct {
	DBAssetID            uint32
	ProductName          string
	TimePerUnitInSeconds float64
}
type addProduct struct {
	ProductName          string  `json:"product_id"`
	TimePerUnitInSeconds float64 `json:"time_per_unit_in_seconds"`
}

// ProcessAddProduct adds a new product to the database
func ProcessAddProduct(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload addProduct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := addProductQueue{
		DBAssetID:            DBassetID,
		ProductName:          parsedPayload.ProductName,
		TimePerUnitInSeconds: parsedPayload.TimePerUnitInSeconds,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.AddProduct, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type addOrderQueue struct {
	DBAssetID   uint32
	ProductName string
	OrderName   string
	TargetUnits uint32
	ProductID   int32
}
type addOrder struct {
	ProductName string `json:"product_id"`
	OrderName   string `json:"order_id"`
	TargetUnits uint32 `json:"target_units"`
}

// ProcessAddOrder adds a new order without begin and end timestamp to the database
func ProcessAddOrder(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload addOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	productID, err := GetProductID(DBassetID, parsedPayload.ProductName)
	if err == sql.ErrNoRows {
		zap.S().Errorf("Product does not exist yet", DBassetID, parsedPayload.ProductName, parsedPayload.OrderName)
		return ErrTryLater
	} else if err != nil { // never executed
		PQErrorHandling("GetProductID db.QueryRow()", err)
	}

	newObject := addOrderQueue{
		DBAssetID:   DBassetID,
		ProductName: parsedPayload.ProductName,
		OrderName:   parsedPayload.OrderName,
		TargetUnits: parsedPayload.TargetUnits,
		ProductID:   productID,
	}
	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.AddOrder, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}

	return nil
}

type startOrderQueue struct {
	DBAssetID   uint32
	TimestampMs uint32
	OrderName   string
}
type startOrder struct {
	TimestampMs uint32 `json:"timestamp_ms"`
	OrderName   string `json:"order_id"`
}

// ProcessStartOrder starts an order by adding beginTimestamp
func ProcessStartOrder(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload startOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := startOrderQueue{
		TimestampMs: parsedPayload.TimestampMs,
		OrderName:   parsedPayload.OrderName,
		DBAssetID:   DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.StartOrder, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type endOrderQueue struct {
	DBAssetID   uint32
	TimestampMs uint32
	OrderName   string
}
type endOrder struct {
	TimestampMs uint32 `json:"timestamp_ms"`
	OrderName   string `json:"order_id"`
}

// ProcessEndOrder starts an order by adding endTimestamp
func ProcessEndOrder(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload endOrder

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := endOrderQueue{
		TimestampMs: parsedPayload.TimestampMs,
		OrderName:   parsedPayload.OrderName,
		DBAssetID:   DBassetID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.EndOrder, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type recommendationStruct struct {
	UID                  string
	TimestampMs          uint32 `json:"timestamp_ms"`
	Customer             string
	Location             string
	Asset                string
	RecommendationType   int32
	Enabled              bool
	RecommendationValues string
	DiagnoseTextDE       string
	DiagnoseTextEN       string
	RecommendationTextDE string
	RecommendationTextEN string
}

// ProcessRecommendationData processes an incoming count message
func ProcessRecommendationData(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload recommendationStruct

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	marshal, err := json.Marshal(parsedPayload)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.Recommendation, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type processValueQueue struct {
	DBAssetID   uint32
	TimestampMs uint32
	Name        string
	Value       int32
}

type processValueFloat64Queue struct {
	DBAssetID   uint32
	TimestampMs uint32
	Name        string
	Value       float64
}

// ProcessProcessValueData processes an incoming processValue message
func ProcessProcessValueData(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload interface{}

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	// process unknown data structure according to https://blog.golang.org/json
	m := parsedPayload.(map[string]interface{})

	if val, ok := m["timestamp_ms"]; ok { //if timestamp_ms key exists (https://stackoverflow.com/questions/2050391/how-to-check-if-a-map-contains-a-key-in-go)
		timestampMs, ok := val.(uint32)
		if !ok {
			timestampMsFloat, ok2 := val.(float64)
			if !ok2 {
				zap.S().Errorf("Timestamp not int64 nor float64", payload, val)
				return nil
			}
			if timestampMsFloat < 0 {
				zap.S().Errorf("Timestamp is negative !", payload, val)
				return nil
			}
			timestampMs = uint32(timestampMsFloat)
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
					newObject := processValueFloat64Queue{
						DBAssetID:   DBassetID,
						TimestampMs: timestampMs,
						Name:        k,
						Value:       valueFloat64,
					}
					marshal, err := json.Marshal(newObject)
					if err != nil {
						return nil
					}

					err = addNewItemToQueue(pg, Prefix.ProcessValueFloat64, marshal)
					if err != nil {
						zap.S().Errorf("Error enqueuing", err)
						return nil
					}
					break
				}
				newObject := processValueQueue{
					DBAssetID:   DBassetID,
					TimestampMs: timestampMs,
					Name:        k,
					Value:       value,
				}
				marshal, err := json.Marshal(newObject)
				if err != nil {
					return nil
				}

				err = addNewItemToQueue(pg, Prefix.ProcessValue, marshal)
				if err != nil {
					zap.S().Errorf("Error enqueuing", err)
					return nil
				}
			}
		}

	}
	return nil
}

type productTagQueue struct {
	DBAssetID   uint32
	TimestampMs uint32  `json:"timestamp_ms"`
	AID         string  `json:"AID"`
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
}

type productTag struct {
	TimestampMs uint32  `json:"timestamp_ms"`
	AID         string  `json:"AID"`
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
}

// ProcessProductTag adds a new productTag to the database
func ProcessProductTag(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload productTag

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := productTagQueue{
		DBAssetID:   DBassetID,
		TimestampMs: parsedPayload.TimestampMs,
		AID:         parsedPayload.AID,
		Name:        parsedPayload.Name,
		Value:       parsedPayload.Value,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.ProductTag, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type productTagStringQueue struct {
	DBAssetID   uint32
	TimestampMs uint32 `json:"timestamp_ms"`
	AID         string `json:"AID"`
	Name        string `json:"name"`
	Value       string `json:"value"`
}

type productTagString struct {
	TimestampMs uint32 `json:"timestamp_ms"`
	AID         string `json:"AID"`
	Name        string `json:"name"`
	Value       string `json:"value"`
}

// ProcessProductTagString adds a new productTagString to the database
func ProcessProductTagString(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload productTagString

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := productTagStringQueue{
		DBAssetID:   DBassetID,
		TimestampMs: parsedPayload.TimestampMs,
		AID:         parsedPayload.AID,
		Name:        parsedPayload.Name,
		Value:       parsedPayload.Value,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.ProductTagString, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type addParentToChildQueue struct {
	DBAssetID   uint32
	TimestampMs uint32 `json:"timestamp_ms"`
	ChildAID    string `json:"childAID"`
	ParentAID   string `json:"parentAID"`
}

type addParentToChild struct {
	TimestampMs uint32 `json:"timestamp_ms"`
	ChildAID    string `json:"childAID"`
	ParentAID   string `json:"parentAID"`
}

// ProcessAddParentToChild adds a new AddParentToChild to the database
func ProcessAddParentToChild(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload addParentToChild

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := addParentToChildQueue{
		DBAssetID:   DBassetID,
		TimestampMs: parsedPayload.TimestampMs,
		ChildAID:    parsedPayload.ChildAID,
		ParentAID:   parsedPayload.ParentAID,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.AddParentToChild, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type modifyStateQueue struct {
	DBAssetID        uint32
	StartTimeStampMs uint32
	EndTimeStampMs   uint32
	NewState         uint32
}

type modifyState struct {
	StartTimeStampMs uint32 `json:"start_time_stamp"`
	EndTimeStampMs   uint32 `json:"end_time_stamp"`
	NewState         uint32 `json:"new_state"`
}

func ProcessModifyState(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload modifyState

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := modifyStateQueue{
		DBAssetID:        DBassetID,
		StartTimeStampMs: parsedPayload.StartTimeStampMs,
		EndTimeStampMs:   parsedPayload.EndTimeStampMs,
		NewState:         parsedPayload.NewState,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.ModifyState, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type deleteShiftByIdQueue struct {
	DBAssetID uint32
	ShiftId   uint32 `json:"shift_id"`
}

type deleteShiftById struct {
	ShiftId uint32 `json:"shift_id"`
}

func ProcessDeleteShiftById(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload deleteShiftById

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := deleteShiftByIdQueue{
		DBAssetID: DBassetID,
		ShiftId:   parsedPayload.ShiftId,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.DeleteShiftById, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type deleteShiftByAssetIdAndBeginTimestampQueue struct {
	DBAssetID        uint32
	BeginTimeStampMs uint32 `json:"begin_time_stamp"`
}

type deleteShiftByAssetIdAndBeginTimestamp struct {
	BeginTimeStampMs uint32 `json:"begin_time_stamp"`
}

func ProcessDeleteShiftByAssetIdAndBeginTime(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload deleteShiftByAssetIdAndBeginTimestamp

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := deleteShiftByAssetIdAndBeginTimestampQueue{
		DBAssetID:        DBassetID,
		BeginTimeStampMs: parsedPayload.BeginTimeStampMs,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.DeleteShiftByAssetIdAndBeginTimestamp, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type modifyProducesPieceQueue struct {
	DBAssetID uint32
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Count int32 `json:"count"`
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Scrap int32 `json:"scrap"`
}

type modifyProducesPiece struct {
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Count int32 `json:"count"`
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Scrap int32 `json:"scrap"`
}

func ProcessModifyProducesPiece(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	// pt.Scrap is -1, if not modified by user
	// pt.Count is -1, if not modified by user
	parsedPayload := modifyProducesPiece{
		Count: -1,
		Scrap: -1,
	}

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)
	newObject := modifyProducesPieceQueue{
		DBAssetID: DBassetID,
		Count:     parsedPayload.Count,
		Scrap:     parsedPayload.Scrap,
	}

	marshal, err := json.Marshal(newObject)
	if err != nil {
		return nil
	}

	err = addNewItemToQueue(pg, Prefix.ModifyProducesPieces, marshal)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return nil
	}
	return nil
}

type processValueStringQueue struct {
	DBAssetID   uint32
	TimestampMs uint32
	Name        string
	Value       string
}

// ProcessProcessValueString adds a new processValueString to the database
func ProcessProcessValueString(customerID string, location string, assetID string, payloadType string, payload []byte, pg *goque.PriorityQueue) error {

	var parsedPayload interface{}

	err := json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		zap.S().Errorf("json.Unmarshal failed", err, payload)
		return nil
	}

	DBassetID := GetAssetID(customerID, location, assetID)

	// process unknown data structure according to https://blog.golang.org/json
	m := parsedPayload.(map[string]interface{})

	if val, ok := m["timestamp_ms"]; ok { //if timestamp_ms key exists (https://stackoverflow.com/questions/2050391/how-to-check-if-a-map-contains-a-key-in-go)
		timestampMs, ok := val.(uint32)
		if !ok {
			timestampMsFloat, ok2 := val.(float64)
			if !ok2 {
				zap.S().Errorf("Timestamp not int64 nor float64", payload, val)
				return nil
			}
			if timestampMsFloat < 0 {
				zap.S().Errorf("Timestamp is negative !", payload, val)
				return nil
			}
			timestampMs = uint32(timestampMsFloat)
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
					return nil
				}

				err = addNewItemToQueue(pg, Prefix.ProcessValue, marshal)
				if err != nil {
					zap.S().Errorf("Error enqueuing", err)
					return nil
				}
			}
		}

	}
	return nil
}
