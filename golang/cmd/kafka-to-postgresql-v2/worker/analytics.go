package worker

import (
	"errors"
	"github.com/goccy/go-json"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
)

func parseWorkOrderCreate(value []byte) (*shared.WorkOrderCreateMessage, error) {
	// Try parse to WorkOrderCreateMessage
	var message shared.WorkOrderCreateMessage
	err := json.Unmarshal(value, &message)

	// Validate that ExternalWorkOrderId, Product.ExternalProductId & Quantity are set
	if message.ExternalWorkOrderId == "" {
		return nil, errors.New("external_work_order_id is required")
	}
	if message.Product.ExternalProductId == "" {
		return nil, errors.New("product.externalProductId is required")
	}
	if message.Quantity == 0 {
		zap.S().Debugf("%+v", message)
		return nil, errors.New("quantity is required to be greater than 0")
	}
	if int(message.Status) > int(shared.Completed) {
		return nil, errors.New("status must be 0, 1 or 2")
	}
	// Status falls back to zero, if not set

	return &message, err
}

func parseWorkOrderStart(value []byte) (*shared.WorkOrderStartMessage, error) {
	// Try parse to WorkOrderStartMessage
	var message shared.WorkOrderStartMessage
	err := json.Unmarshal(value, &message)

	// Validate that ExternalWorkOrderId & StartTimeUnixMs are set
	if message.ExternalWorkOrderId == "" {
		return nil, errors.New("external_work_order_id is required")
	}
	if message.StartTimeUnixMs == 0 {
		return nil, errors.New("start_time_unix_ms is required")
	}
	return &message, err
}

func parseWorkOrderStop(value []byte) (*shared.WorkOrderStopMessage, error) {
	// Try parse to WorkOrderStopMessage
	var message shared.WorkOrderStopMessage
	err := json.Unmarshal(value, &message)

	// Validate that ExternalWorkOrderId & EndTimeUnixMs are set
	if message.ExternalWorkOrderId == "" {
		return nil, errors.New("external_work_order_id is required")
	}
	if message.EndTimeUnixMs == 0 {
		return nil, errors.New("end_time_unix_ms is required")
	}
	return &message, err
}

func parseProductAdd(value []byte) (*shared.ProductAddMessage, error) {
	// Try parse to ProductAddMessage
	var message shared.ProductAddMessage
	err := json.Unmarshal(value, &message)

	// Validate that ExternalProductId, EndTimeUnixMs, Quantity are set
	if message.ExternalProductId == "" {
		return nil, errors.New("externalProductId is required")
	}
	if message.EndTimeUnixMs == 0 {
		return nil, errors.New("end_time is required")
	}
	if message.Quantity == 0 {
		return nil, errors.New("quantity is required")
	}
	return &message, err
}

func parseProductSetBadQuantity(value []byte) (*shared.ProductSetBadQuantityMessage, error) {
	// Try parse to ProductSetBadQuantityMessage
	var message shared.ProductSetBadQuantityMessage
	err := json.Unmarshal(value, &message)

	// Validate that ExternalProductId & BadQuantity are set
	if message.ExternalProductId == "" {
		return nil, errors.New("externalProductId is required")
	}
	if message.BadQuantity == 0 {
		return nil, errors.New("badQuantity is required")
	}
	return &message, err

}

func parseProductTypeCreate(value []byte) (*shared.ProductTypeCreateMessage, error) {
	// Try parse to ProductTypeCreateMessage
	var message shared.ProductTypeCreateMessage
	err := json.Unmarshal(value, &message)

	// Validate that ExternalProductTypeId & CycleTimeMs are set
	if message.ExternalProductTypeId == "" {
		return nil, errors.New("external_product_type_id is required")
	}
	if message.CycleTimeMs == 0 {
		return nil, errors.New("cycleTimeMs is required")
	}
	return &message, err
}

func parseShiftAdd(value []byte) (*shared.ShiftAddMessage, error) {
	// Try parse to ShiftAddMessage
	var message shared.ShiftAddMessage
	err := json.Unmarshal(value, &message)

	// Validate that StartTimeUnixMs & EndTimeUnixMs are set
	if message.StartTimeUnixMs == 0 {
		return nil, errors.New("start_time_unix_ms is required")
	}
	if message.EndTimeUnixMs == 0 {
		return nil, errors.New("end_time_unix_ms is required")
	}
	return &message, err
}

func parseShiftDelete(value []byte) (*shared.ShiftDeleteMessage, error) {
	// Try parse to ShiftDeleteMessage
	var message shared.ShiftDeleteMessage
	err := json.Unmarshal(value, &message)

	// Validate that StartTimeUnixMs is set
	if message.StartTimeUnixMs == 0 {
		return nil, errors.New("start_time_unix_ms is required")
	}
	return &message, err
}

func parseStateAdd(value []byte) (*shared.StateAddMessage, error) {
	// Try parse to StateAddMessage
	var message shared.StateAddMessage
	err := json.Unmarshal(value, &message)

	// Validate that State & StartTimeUnixMs are set
	if message.State == 0 {
		return nil, errors.New("state is required")
	}
	if message.StartTimeUnixMs == 0 {
		return nil, errors.New("start_time_unix_ms is required")
	}
	return &message, err
}

func parseStateOverwrite(value []byte) (*shared.StateOverwriteMessage, error) {
	// Try parse to StateOverwriteMessage
	var message shared.StateOverwriteMessage
	err := json.Unmarshal(value, &message)

	// Validate that State, EndTimeUnixMs & StartTimeUnixMs are set
	if message.State == 0 {
		return nil, errors.New("state is required")
	}
	if message.EndTimeUnixMs == 0 {
		return nil, errors.New("end_time_unix_ms is required")
	}
	if message.StartTimeUnixMs == 0 {
		return nil, errors.New("start_time_unix_ms is required")
	}
	return &message, err
}
