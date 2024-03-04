package worker

import (
	"github.com/stretchr/testify/assert"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"testing"
)

func TestParseWorkOrderCreate(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "product":{"externalProductId": "sample"}, "quantity": 10}`
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
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "product":{"externalProductId": "sample", "cycleTimeMs": 10}, "quantity": 10, "startTimeUnixMs": 100, "endTimeUnixMs": 200}`
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
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "product":{"externalProductId": "sample"}, "quantity": -10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-negative-cycle-time", func(t *testing.T) {
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "product":{"externalProductId": "sample", "cycleTimeMs": -10}, "quantity": 10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-negative-start-time", func(t *testing.T) {
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "product":{"externalProductId": "sample"}, "quantity": 10, "startTimeUnixMs": -100}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-negative-end-time", func(t *testing.T) {
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "product":{"externalProductId": "sample"}, "quantity": 10, "endTimeUnixMs": -100}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-no-external-work-order-id", func(t *testing.T) {
		workOrderCreateJson := `{"product":{"externalProductId": "sample"}, "quantity": 10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-no-external-product-id", func(t *testing.T) {
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "quantity": 10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-no-quantity", func(t *testing.T) {
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "product":{"externalProductId": "sample"}}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("disallow-no-product", func(t *testing.T) {
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "quantity": 10}`
		_, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.Error(t, err)
	})
	t.Run("status-fallback-zero", func(t *testing.T) {
		workOrderCreateJson := `{"externalWorkOrderId": "#1278", "product":{"externalProductId": "sample"}, "quantity": 10}`
		workOrderCreate, err := parseWorkOrderCreate([]byte(workOrderCreateJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1278", workOrderCreate.ExternalWorkOrderId)
		assert.Equal(t, uint64(10), workOrderCreate.Quantity)
		assert.Equal(t, "sample", workOrderCreate.Product.ExternalProductId)
		assert.Equal(t, shared.Planned, workOrderCreate.Status)
	})
}

func TestParseWorkOrderStart(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		workOrderStartJson := `{"externalWorkOrderId": "#1278", "startTimeUnixMs": 100}`
		workOrderStart, err := parseWorkOrderStart([]byte(workOrderStartJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1278", workOrderStart.ExternalWorkOrderId)
		assert.Equal(t, uint64(100), workOrderStart.StartTimeUnixMs)
	})

	t.Run("disallow-no-external-work-order-id", func(t *testing.T) {
		workOrderStartJson := `{"startTimeUnixMs": 100}`
		_, err := parseWorkOrderStart([]byte(workOrderStartJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-start-time", func(t *testing.T) {
		workOrderStartJson := `{"externalWorkOrderId": "#1278"}`
		_, err := parseWorkOrderStart([]byte(workOrderStartJson))
		assert.Error(t, err)
	})
}

func TestParseWorkOrderStop(t *testing.T) {
	t.Run("from-string-only-required", func(t *testing.T) {
		workOrderStopJson := `{"externalWorkOrderId": "#1278", "endTimeUnixMs": 100}`
		workOrderEnd, err := parseWorkOrderStop([]byte(workOrderStopJson))
		assert.NoError(t, err)
		assert.Equal(t, "#1278", workOrderEnd.ExternalWorkOrderId)
		assert.Equal(t, uint64(100), workOrderEnd.EndTimeUnixMs)
	})

	t.Run("disallow-no-external-work-order-id", func(t *testing.T) {
		workOrderStopJson := `{"endTimeUnixMs": 100}`
		_, err := parseWorkOrderStop([]byte(workOrderStopJson))
		assert.Error(t, err)
	})

	t.Run("disallow-no-end-time", func(t *testing.T) {
		workOrderStopJson := `{"externalWorkOrderId": "#1278"}`
		_, err := parseWorkOrderStop([]byte(workOrderStopJson))
		assert.Error(t, err)
	})
}
