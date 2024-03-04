package postgresql

import (
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"testing"
)

func TestWorkOrder(t *testing.T) {
	c := CreateMockConnection(t)
	defer c.db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.db.(pgxmock.PgxPoolIface)
	assert.True(t, ok)

	t.Run("create", func(t *testing.T) {
		msg := sharedStructs.WorkOrderCreateMessage{
			ExternalWorkOrderId: "#1274",
			Product: sharedStructs.WorkOrderCreateMessageProduct{
				ExternalProductId: "test",
				CycleTimeMs:       120,
			},
			Quantity:        0,
			Status:          0,
			StartTimeUnixMs: 0,
			EndTimeUnixMs:   0,
		}
		topic := sharedStructs.TopicDetails{
			Enterprise: "umh",
			Tag:        "work-order.create",
		}

		// Expect Query from GetOrInsertAsset
		mock.ExpectQuery(`SELECT id FROM asset WHERE enterprise = \$1 AND site = \$2 AND area = \$3 AND line = \$4 AND workcell = \$5 AND origin_id = \$6`).
			WithArgs("umh", "", "", "", "", "").
			WillReturnRows(mock.NewRows([]string{"id"}).AddRow(1))

		// Expect Query from GetOrInsertProduct
		mock.ExpectQuery(`SELECT productTypeId FROM product_types WHERE externalProductTypeId = \$1 AND assetId = \$2`).
			WithArgs("test", 1).
			WillReturnRows(mock.NewRows([]string{"productTypeId"}).AddRow(1))

		// Expect Exec from InsertWorkOrderCreate
		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`
		INSERT INTO work_orders \(externalWorkOrderId, assetId, productTypeId, quantity, status, startTime, endTime\) VALUES \(\$1, \$2, \$3, \$4, \$5, to_timestamp\(\$6\/1000\), to_timestamp\(\$7\/1000\)\)
	`).WithArgs("#1274", 1, 1, uint64(0), int(0), uint64(0), uint64(0)).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()

		err := c.InsertWorkOrderCreate(&msg, &topic)
		assert.NoError(t, err)
	})

	t.Run("start", func(t *testing.T) {
		msg := sharedStructs.WorkOrderStartMessage{
			ExternalWorkOrderId: "#1274",
			StartTimeUnixMs:     0,
		}
		topic := sharedStructs.TopicDetails{
			Enterprise: "umh",
			Tag:        "work-order.start",
		}

		// Expect Exec from UpdateWorkOrderSetStart
		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`
		UPDATE work_orders
		SET status = 1, startTime = to_timestamp\(\$2 \/ 1000\)
		WHERE externalWorkOrderId = \$1 AND assetId = \$3
	`).WithArgs("#1274", uint64(0), 1).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit()

		err := c.UpdateWorkOrderSetStart(&msg, &topic)
		assert.NoError(t, err)
	})

	t.Run("end", func(t *testing.T) {
		msg := sharedStructs.WorkOrderStopMessage{
			ExternalWorkOrderId: "#1274",
			EndTimeUnixMs:       0,
		}
		topic := sharedStructs.TopicDetails{
			Enterprise: "umh",
			Tag:        "work-order.stop",
		}

		// Expect Exec from UpdateWorkOrderSetStop
		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`
		UPDATE work_orders
		SET status = 2, endTime = to_timestamp\(\$2 \/ 1000\)
		WHERE externalWorkOrderId = \$1 AND assetId = \$3
		`).WithArgs("#1274", uint64(0), 1).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit()

		err := c.UpdateWorkOrderSetStop(&msg, &topic)
		assert.NoError(t, err)

	})
}

func TestProduct(t *testing.T) {
	c := CreateMockConnection(t)
	defer c.db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.db.(pgxmock.PgxPoolIface)
	assert.True(t, ok)

	// Insert mock product type
	mock.ExpectQuery(`SELECT productTypeId FROM product_types WHERE externalProductTypeId = \$1 AND assetId = \$2`).
		WithArgs("#1274", 1).
		WillReturnRows(mock.NewRows([]string{"productTypeId"}).AddRow(1))
	_, err := c.GetOrInsertProductType(1, "#1274", 1)
	assert.NoError(t, err)

	t.Run("add", func(t *testing.T) {
		msg := sharedStructs.ProductAddMessage{
			ExternalProductId: "#1274",
			ProductBatchId:    "0000-1234",
			StartTimeUnixMs:   0,
			EndTimeUnixMs:     10,
			Quantity:          512,
			BadQuantity:       0,
		}
		topic := sharedStructs.TopicDetails{
			Enterprise: "umh",
			Tag:        "product.add",
		}

		// Expect Query from GetOrInsertAsset
		mock.ExpectQuery(`SELECT id FROM asset WHERE enterprise = \$1 AND site = \$2 AND area = \$3 AND line = \$4 AND workcell = \$5 AND origin_id = \$6`).
			WithArgs("umh", "", "", "", "", "").
			WillReturnRows(mock.NewRows([]string{"id"}).AddRow(1))

		// Expect Exec from InsertProductAdd
		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`INSERT INTO products \(externalProductTypeId, productBatchId, assetId, startTime, endTime, quantity, badQuantity\)
		VALUES \(\$1, \$2, \$3, to_timestamp\(\$4\/1000\), to_timestamp\(\$5\/1000\), \$6, \$7\)`).
			WithArgs(1, "0000-1234", 1, uint64(0), uint64(10), 512, 0).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()

		err := c.InsertProductAdd(&msg, &topic)
		assert.NoError(t, err)
	})
}

func TestProductType(t *testing.T) {
	c := CreateMockConnection(t)
	defer c.db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.db.(pgxmock.PgxPoolIface)
	assert.True(t, ok)

	t.Run("create", func(t *testing.T) {
		msg := sharedStructs.ProductTypeCreateMessage{
			ExternalProductTypeId: "#1275",
			CycleTimeMs:           512,
		}
		topic := sharedStructs.TopicDetails{
			Enterprise: "umh",
			Tag:        "product-type.create",
		}

		// Expect Query from GetOrInsertAsset
		mock.ExpectQuery(`SELECT id FROM asset WHERE enterprise = \$1 AND site = \$2 AND area = \$3 AND line = \$4 AND workcell = \$5 AND origin_id = \$6`).
			WithArgs("umh", "", "", "", "", "").
			WillReturnRows(mock.NewRows([]string{"id"}).AddRow(1))

		// Expect Exec from InsertProductTypeCreate
		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`INSERT INTO product_types \(externalProductTypeId, cycleTime, assetId\)
		VALUES \(\$1, \$2, \$3\)
		ON CONFLICT \(externalProductTypeId, assetId\) DO NOTHING`).
			WithArgs("#1275", 512, 1).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		mock.ExpectCommit()

		err := c.InsertProductTypeCreate(&msg, &topic)
		assert.NoError(t, err)
	})
}

func TestShift(t *testing.T) {
	c := CreateMockConnection(t)
	defer c.db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.db.(pgxmock.PgxPoolIface)
	assert.True(t, ok)

	t.Run("add", func(t *testing.T) {
		msg := sharedStructs.ShiftAddMessage{
			StartTimeUnixMs: 1,
			EndTimeUnixMs:   2,
		}

		topic := sharedStructs.TopicDetails{
			Enterprise: "umh",
			Tag:        "shift.add",
		}

		// Expect Query from GetOrInsertAsset
		mock.ExpectQuery(`SELECT id FROM asset WHERE enterprise = \$1 AND site = \$2 AND area = \$3 AND line = \$4 AND workcell = \$5 AND origin_id = \$6`).
			WithArgs("umh", "", "", "", "", "").
			WillReturnRows(mock.NewRows([]string{"id"}).AddRow(1))

		// Expect Exec from InsertShiftAdd
		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`INSERT INTO shifts \(assetId, startTime, endTime\)
		VALUES \(\$1, to_timestamp\(\$2 \/ 1000\), to_timestamp\(\$3 \/ 1000\)\)
		ON CONFLICT ON CONSTRAINT shift_start_asset_uniq
		DO NOTHING;`).WithArgs(1, uint64(1), uint64(2)).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		mock.ExpectCommit()

		err := c.InsertShiftAdd(&msg, &topic)
		assert.NoError(t, err)
	})

	t.Run("delete", func(t *testing.T) {
		msg := sharedStructs.ShiftDeleteMessage{
			StartTimeUnixMs: 1,
		}

		topic := sharedStructs.TopicDetails{
			Enterprise: "umh",
			Tag:        "shift.delete",
		}

		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`DELETE FROM shifts WHERE assetId = \$1 AND startTime = to_timestamp\(\$2 \/ 1000\)`).
			WithArgs(1, uint64(1)).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))

		mock.ExpectCommit()

		err := c.DeleteShiftByStartTime(&msg, &topic)
		assert.NoError(t, err)
	})
}
