package worker

import (
	"github.com/stretchr/testify/assert"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"testing"
)

func TestParseWorkOrderCreate(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "product":{"external_product_id": "sample"}, "quantity": 10}`
		workOrderCreate, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1278", workOrderCreate.ExternalWorkOrderId)
		assert.Equal(t, uint64(10), workOrderCreate.Quantity)
		assert.Equal(t, "sample", workOrderCreate.Product.ExternalProductId)
		assert.Zerof(t, workOrderCreate.Product.CycleTimeMs, "cycle time should be zero")
		assert.Zerof(t, workOrderCreate.StartTimeUnixMs, "start time should be zero")
		assert.Zerof(t, workOrderCreate.EndTimeUnixMs, "end time should be zero")
	})
	t.Run("from-string-only-full", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "product":{"external_product_id": "sample", "cycle_time_ms": 10}, "quantity": 10, "start_time_unix_ms": 100, "end_time_unix_ms": 200}`
		workOrderCreate, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1278", workOrderCreate.ExternalWorkOrderId)
		assert.Equal(t, uint64(10), workOrderCreate.Quantity)
		assert.Equal(t, "sample", workOrderCreate.Product.ExternalProductId)
		assert.Equal(t, uint64(10), workOrderCreate.Product.CycleTimeMs)
		assert.Equal(t, uint64(100), workOrderCreate.StartTimeUnixMs)
		assert.Equal(t, uint64(200), workOrderCreate.EndTimeUnixMs)
	})
	t.Run("disallow-negative-quantity", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "product":{"external_product_id": "sample"}, "quantity": -10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-negative-cycle-time", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "product":{"external_product_id": "sample", "cycle_time_ms": -10}, "quantity": 10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-negative-start-time", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "product":{"external_product_id": "sample"}, "quantity": 10, "start_time_unix_ms": -100}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-negative-end-time", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "product":{"external_product_id": "sample"}, "quantity": 10, "end_time_unix_ms": -100}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-no-external-work-order-id", func(t *testing.T) {
		workOrderCreateJson := `{"product":{"external_product_id": "sample"}, "quantity": 10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-no-external-product-id", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "quantity": 10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-no-quantity", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "product":{"external_product_id": "sample"}}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-no-product", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "quantity": 10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("status-fallback-zero", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "product":{"external_product_id": "sample"}, "quantity": 10}`
		workOrderCreate, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1278", workOrderCreate.ExternalWorkOrderId)
		assert.Equal(t, uint64(10), workOrderCreate.Quantity)
		assert.Equal(t, "sample", workOrderCreate.Product.ExternalProductId)
		assert.Equal(t, shared.Planned, workOrderCreate.Status)
	})
	t.Run("check-out-of-range-status", func(t *testing.T) {
		workOrderCreateJson := `{"external_work_order_id": "#1278", "product":{"external_product_id": "sample"}, "quantity": 10, "status": 100}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
}

func TestParseWorkOrderStart(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		workOrderStartJson := `{"external_work_order_id": "#1278", "start_time_unix_ms": 100}`
		workOrderStart, err := parseWorkOrderStart([]byte(workOrderStartJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1278", workOrderStart.ExternalWorkOrderId)
		assert.Equal(t, uint64(100), workOrderStart.StartTimeUnixMs)
	})

	t.Run("disallow-no-external-work-order-id", func(t *testing.T) {
		workOrderStartJson := `{"start_time_unix_ms": 100}`
		_, err := parseWorkOrderStart([]byte(workOrderStartJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-start-time", func(t *testing.T) {
		workOrderStartJson := `{"external_work_order_id": "#1278"}`
		_, err := parseWorkOrderStart([]byte(workOrderStartJson))
		assert.Error(t, err)
	})
}

func TestParseWorkOrderStop(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		workOrderStopJson := `{"external_work_order_id": "#1278", "end_time_unix_ms": 100}`
		workOrderEnd, err := parseWorkOrderStop([]byte(workOrderStopJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1278", workOrderEnd.ExternalWorkOrderId)
		assert.Equal(t, uint64(100), workOrderEnd.EndTimeUnixMs)
	})

	t.Run("disallow-no-external-work-order-id", func(t *testing.T) {
		workOrderStopJson := `{"end_time_unix_ms": 100}`
		_, err := parseWorkOrderStop([]byte(workOrderStopJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-end-time", func(t *testing.T) {
		workOrderStopJson := `{"external_work_order_id": "#1278"}`
		_, err := parseWorkOrderStop([]byte(workOrderStopJson))
		assert.Error(t, err)
	})
}

func TestParseProductAdd(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		productAddJson := `{"external_product_id": "#1274", "end_time_unix_ms": 100, "quantity": 10}`
		productAdd, err := parseProductAdd([]byte(productAddJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1274", productAdd.ExternalProductId)
		assert.Equal(t, uint64(100), productAdd.EndTimeUnixMs)
	})

	t.Run("disallow-no-external-product-id", func(t *testing.T) {
		productAddJson := `{"end_time_unix_ms": 100}`
		_, err := parseProductAdd([]byte(productAddJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-end-time", func(t *testing.T) {
		productAddJson := `{"external_product_id": "#1274"}`
		_, err := parseProductAdd([]byte(productAddJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-quantity", func(t *testing.T) {
		productAddJson := `{"external_product_id": "#1274", "end_time_unix_ms": 100}`
		_, err := parseProductAdd([]byte(productAddJson))
		assert.Error(t, err)
	})
}

func TestParseProductSetBadQuantity(t *testing.T) {
	t.Run("disallow-negative-quantity", func(t *testing.T) {
		productSetJson := `{"external_product_id": "#1274", "quantity": -10}`
		_, err := parseProductSetBadQuantity([]byte(productSetJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-external-product-id", func(t *testing.T) {
		productSetJson := `{"quantity": 10}`
		_, err := parseProductSetBadQuantity([]byte(productSetJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-quantity", func(t *testing.T) {
		productSetJson := `{"external_product_id": "#1274"}`
		_, err := parseProductSetBadQuantity([]byte(productSetJson))
		assert.Error(t, err)
	})
}

func TestProductTypeCreate(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		productTypeCreateJson := `{"external_product_type_id": "#1274", "cycle_time_ms": 100}`
		productTypeCreate, err := parseProductTypeCreate([]byte(productTypeCreateJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1274", productTypeCreate.ExternalProductTypeId)
		assert.Equal(t, uint64(100), productTypeCreate.CycleTimeMs)
	})

	t.Run("disallow-no-external-product-type-id", func(t *testing.T) {
		productTypeCreateJson := `{"cycle_time_ms": 100}`
		_, err := parseProductTypeCreate([]byte(productTypeCreateJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-cycle-time", func(t *testing.T) {
		productTypeCreateJson := `{"external_product_type_id": "#1274"}`
		_, err := parseProductTypeCreate([]byte(productTypeCreateJson))
		assert.Error(t, err)
	})
}

func TestShiftAdd(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		shiftAddJson := `{"start_time_unix_ms": 100, "end_time_unix_ms": 200}`
		shiftAdd, err := parseShiftAdd([]byte(shiftAddJson))
		assert.NoError(t, err)
		assert.Equal(t, uint64(100), shiftAdd.StartTimeUnixMs)
		assert.Equal(t, uint64(200), shiftAdd.EndTimeUnixMs)
	})

	t.Run("disallow-no-start-time", func(t *testing.T) {
		shiftAddJson := `{"external_shift_id": "#1274", "end_time_unix_ms": 200}`
		_, err := parseShiftAdd([]byte(shiftAddJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-end-time", func(t *testing.T) {
		shiftAddJson := `{"external_shift_id": "#1274", "start_time_unix_ms": 100}`
		_, err := parseShiftAdd([]byte(shiftAddJson))
		assert.Error(t, err)
	})
}

func TestShiftDelete(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		shiftDeleteJson := `{"start_time_unix_ms": 100}`
		shiftDelete, err := parseShiftDelete([]byte(shiftDeleteJson))
		assert.NoError(t, err)
		assert.Equal(t, uint64(100), shiftDelete.StartTimeUnixMs)
	})

	t.Run("disallow-no-start-time", func(t *testing.T) {
		shiftDeleteJson := `{"external_shift_id": "#1274"}`
		_, err := parseShiftDelete([]byte(shiftDeleteJson))
		assert.Error(t, err)
	})
}

func TestStateAdd(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		stateAddJson := `{"state": 10000, "start_time_unix_ms": 100}`
		stateAdd, err := parseStateAdd([]byte(stateAddJson))
		assert.NoError(t, err)
		assert.Equal(t, uint64(100), stateAdd.StartTimeUnixMs)
		assert.Equal(t, uint64(10000), stateAdd.State)
	})

	t.Run("disallow-no-state", func(t *testing.T) {
		stateAddJson := `{"start_time_unix_ms": 100}`
		_, err := parseStateAdd([]byte(stateAddJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-start-time", func(t *testing.T) {
		stateAddJson := `{"state": 10000}`
		_, err := parseStateAdd([]byte(stateAddJson))
		assert.Error(t, err)
	})
}

func TestStateOverwrite(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		stateOverwriteJson := `{"state": 10000, "start_time_unix_ms": 100, "end_time_unix_ms": 200}`
		stateOverwrite, err := parseStateOverwrite([]byte(stateOverwriteJson))
		assert.NoError(t, err)
		assert.Equal(t, uint64(100), stateOverwrite.StartTimeUnixMs)
		assert.Equal(t, uint64(10000), stateOverwrite.State)
		assert.Equal(t, uint64(200), stateOverwrite.EndTimeUnixMs)
	})

	t.Run("disallow-no-state", func(t *testing.T) {
		stateOverwriteJson := `{"start_time_unix_ms": 100}`
		_, err := parseStateOverwrite([]byte(stateOverwriteJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-start-time", func(t *testing.T) {
		stateOverwriteJson := `{"state": 10000}`
		_, err := parseStateOverwrite([]byte(stateOverwriteJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-end-time", func(t *testing.T) {
		stateOverwriteJson := `{"state": 10000, "start_time_unix_ms": 100}`
		_, err := parseStateOverwrite([]byte(stateOverwriteJson))
		assert.Error(t, err)
	})
}
