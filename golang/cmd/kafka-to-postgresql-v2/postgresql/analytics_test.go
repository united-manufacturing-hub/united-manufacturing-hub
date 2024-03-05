package postgresql

import (
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/helper"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"testing"
)

func TestWorkOrder(t *testing.T) {
	helper.InitTestLogging()
	c := CreateMockConnection(t)
	// Cast to PostgresqlConnection to access the DB field

	defer c.Db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.Db.(pgxmock.PgxPoolIface)
	assert.True(t, ok)

	t.Run("create", func(t *testing.T) {
		msg := sharedStructs.WorkOrderCreateMessage{
			ExternalWorkOrderId: "#1274",
			Product: sharedStructs.WorkOrderCreateMessageProduct{
				ExternalProductId: "test",
				CycleTimeMs:       helper.IntToUint64Ptr(120),
			},
			Quantity:        0,
			Status:          0,
			StartTimeUnixMs: helper.IntToUint64Ptr(0),
			EndTimeUnixMs:   helper.IntToUint64Ptr(0),
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
		mock.ExpectQuery(`SELECT product_type_id FROM product_type WHERE external_product_type_id = \$1 AND asset_id = \$2`).
			WithArgs("test", 1).
			WillReturnRows(mock.NewRows([]string{"product_type_id"}).AddRow(1))

		// Expect Exec from InsertWorkOrderCreate
		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`
		INSERT INTO work_order\(external_work_order_id, asset_id, product_type_id, quantity, status, start_time, end_time\) VALUES \(\$1, \$2, \$3, \$4, \$5, CASE WHEN \$6 IS NOT NULL THEN to_timestamp\(\$6/1000\) END, CASE WHEN \$7 IS NOT NULL THEN to_timestamp\(\$7/1000\) END\)
	`).WithArgs("#1274", 1, 1, 0, 0, helper.Uint64PtrToNullInt64(helper.IntToUint64Ptr(0)), helper.Uint64PtrToNullInt64(helper.IntToUint64Ptr(0))).
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
		UPDATE work_order
		SET status = 1, start_time = to_timestamp\(\$2 \/ 1000\)
		WHERE external_work_order_id = \$1 AND status = 0 AND start_time IS NULL AND asset_id = \$3
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
		UPDATE work_order
		SET status = 2, end_time = to_timestamp\(\$2 \/ 1000\)
		WHERE external_work_order_id = \$1 AND status = 1 AND end_time IS NULL AND asset_id = \$3
		`).WithArgs("#1274", uint64(0), 1).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit()

		err := c.UpdateWorkOrderSetStop(&msg, &topic)
		assert.NoError(t, err)

	})
}

func TestProduct(t *testing.T) {
	c := CreateMockConnection(t)
	// Cast to PostgresqlConnection to access the DB field

	defer c.Db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.Db.(pgxmock.PgxPoolIface)
	assert.True(t, ok)

	// Insert mock product type
	mock.ExpectQuery(`SELECT product_type_id FROM product_type WHERE external_product_type_id = \$1 AND asset_id = \$2`).
		WithArgs("#1274", 1).
		WillReturnRows(mock.NewRows([]string{"product_type_id"}).AddRow(1))
	_, err := c.GetOrInsertProductType(1, "#1274", helper.IntToUint64Ptr(1))
	assert.NoError(t, err)

	t.Run("add", func(t *testing.T) {
		msg := sharedStructs.ProductAddMessage{
			ExternalProductId: "#1274",
			ProductBatchId:    helper.StringToPtr("0000-1234"),
			StartTimeUnixMs:   helper.IntToUint64Ptr(0),
			EndTimeUnixMs:     10,
			Quantity:          512,
			BadQuantity:       helper.IntToUint64Ptr(0),
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
		mock.ExpectExec(`INSERT INTO product\(external_product_type_id, product_batch_id, asset_id, start_time, end_time, quantity, bad_quantity\)
		VALUES \( \$1, \$2, \$3, CASE WHEN \$4 IS NOT NULL THEN to_timestamp\(\$4\/1000\) END, to_timestamp\(\$5\/1000\), \$6, \$7 \)`).
			WithArgs(1, helper.StringToNullString("0000-1234"), 1, helper.Uint64PtrToNullInt64(helper.IntToUint64Ptr(0)), uint64(10), 512, helper.Uint64PtrToNullInt64(helper.IntToUint64Ptr(0))).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()

		err := c.InsertProductAdd(&msg, &topic)
		assert.NoError(t, err)
	})
}

func TestProductType(t *testing.T) {
	c := CreateMockConnection(t)
	// Cast to PostgresqlConnection to access the DB field

	defer c.Db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.Db.(pgxmock.PgxPoolIface)
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
		mock.ExpectExec(`INSERT INTO product_type\(external_product_type_id, cycle_time_ms, asset_id\)
		VALUES \(\$1, \$2, \$3\)
		ON CONFLICT \(external_product_type_id, asset_id\) DO NOTHING`).
			WithArgs("#1275", 512, 1).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		mock.ExpectCommit()

		err := c.InsertProductTypeCreate(&msg, &topic)
		assert.NoError(t, err)
	})
}

func TestShift(t *testing.T) {
	c := CreateMockConnection(t)
	// Cast to PostgresqlConnection to access the DB field

	defer c.Db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.Db.(pgxmock.PgxPoolIface)
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
		mock.ExpectExec(`INSERT INTO shift\(asset_id, start_time, end_time\)
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
		mock.ExpectExec(`DELETE FROM shift WHERE asset_id = \$1 AND start_time = to_timestamp\(\$2 \/ 1000\)`).
			WithArgs(1, uint64(1)).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))

		mock.ExpectCommit()

		err := c.DeleteShiftByStartTime(&msg, &topic)
		assert.NoError(t, err)
	})
}

func TestState(t *testing.T) {
	c := CreateMockConnection(t)
	// Cast to PostgresqlConnection to access the DB field

	defer c.Db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.Db.(pgxmock.PgxPoolIface)
	assert.True(t, ok)

	t.Run("add", func(t *testing.T) {
		msg := sharedStructs.StateAddMessage{
			StartTimeUnixMs: 1,
			State:           10000,
		}

		topic := sharedStructs.TopicDetails{
			Enterprise: "umh",
			Tag:        "state.add",
		}

		// Expect Query from GetOrInsertAsset
		mock.ExpectQuery(`SELECT id FROM asset WHERE enterprise = \$1 AND site = \$2 AND area = \$3 AND line = \$4 AND workcell = \$5 AND origin_id = \$6`).
			WithArgs("umh", "", "", "", "", "").
			WillReturnRows(mock.NewRows([]string{"id"}).AddRow(1))

		// Expect Exec from InsertStateAdd
		mock.ExpectBeginTx(pgx.TxOptions{})

		mock.ExpectExec(`UPDATE state
		SET end_time \= to_timestamp\(\$2\/1000\)
		WHERE asset_id \= \$1
		AND end_time IS NULL
		AND start_time \< to_timestamp\(\$2\/1000\)
		`).WithArgs(1, uint64(1)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		mock.ExpectExec(`INSERT INTO state\(asset_id, start_time, state\)
		VALUES \(\$1, to_timestamp\(\$2\/1000\), \$3\)
		ON CONFLICT ON CONSTRAINT state_start_asset_uniq
		DO NOTHING`).WithArgs(1, uint64(1), 10000).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		mock.ExpectCommit()

		err := c.InsertStateAdd(&msg, &topic)
		assert.NoError(t, err)

		// Let's insert two more states to test the update functionality
		// One starts at 100, the other at 200
		msg = sharedStructs.StateAddMessage{
			StartTimeUnixMs: 100,
			State:           20000,
		}

		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`UPDATE state
		SET end_time \= to_timestamp\(\$2\/1000\)
		WHERE asset_id \= \$1
		AND end_time IS NULL
		AND start_time \< to_timestamp\(\$2\/1000\)
		`).WithArgs(1, uint64(100)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		mock.ExpectExec(`INSERT INTO state\(asset_id, start_time, state\)
		VALUES \(\$1, to_timestamp\(\$2\/1000\), \$3\)
		ON CONFLICT ON CONSTRAINT state_start_asset_uniq
		DO NOTHING`).WithArgs(1, uint64(100), 20000).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		mock.ExpectCommit()

		err = c.InsertStateAdd(&msg, &topic)
		assert.NoError(t, err)

		msg = sharedStructs.StateAddMessage{
			StartTimeUnixMs: 200,
			State:           30000,
		}

		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`UPDATE state
		SET end_time \= to_timestamp\(\$2\/1000\)
		WHERE asset_id \= \$1
		AND end_time IS NULL
		AND start_time \< to_timestamp\(\$2\/1000\)
		`).WithArgs(1, uint64(200)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		mock.ExpectExec(`INSERT INTO state\(asset_id, start_time, state\)
		VALUES \(\$1, to_timestamp\(\$2\/1000\), \$3\)
		ON CONFLICT ON CONSTRAINT state_start_asset_uniq
		DO NOTHING`).WithArgs(1, uint64(200), 30000).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		mock.ExpectCommit()

		err = c.InsertStateAdd(&msg, &topic)
		assert.NoError(t, err)

	})

	t.Run("overwrite", func(t *testing.T) {
		// We now have 3 states, 0-100, 100-200, 200-...
		// Let's test the overwrite by first setting 0-100 to 40000

		msg := sharedStructs.StateOverwriteMessage{
			StartTimeUnixMs: 0,
			EndTimeUnixMs:   100,
			State:           40000,
		}

		topic := sharedStructs.TopicDetails{
			Enterprise: "umh",
			Tag:        "state.overwrite",
		}

		mock.ExpectBeginTx(pgx.TxOptions{})
		// The prev state will be cleanly deleted
		mock.ExpectExec(`DELETE FROM state
		WHERE asset_id = \$1
		AND start_time >= to_timestamp\(\$2\/1000\)
		AND start_time <= to_timestamp\(\$3\/1000\)
		`).WithArgs(1, uint64(0), uint64(100)).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))

		// The left update command will not change anything
		mock.ExpectExec(`UPDATE state
		SET end_time = to_timestamp\(\$2\/1000\)
		WHERE asset_id = \$1
		AND end_time > to_timestamp\(\$2\/1000\)
		AND end_time <= to_timestamp\(\$3\/1000\)
		`).WithArgs(1, uint64(0), uint64(100)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		// The right update command will not change anything
		mock.ExpectExec(`UPDATE state
		SET start_time = to_timestamp\(\$3\/1000\)
		WHERE asset_id = \$1
		AND start_time >= to_timestamp\(\$2\/1000\)
		AND start_time < to_timestamp\(\$3\/1000\)
		`).WithArgs(1, uint64(0), uint64(100)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		// The insert command will insert the new state
		mock.ExpectExec(`INSERT INTO state\(asset_id, start_time, end_time, state\)
		VALUES \(\$1, to_timestamp\(\$2\/1000\), to_timestamp\(\$3\/1000\), \$4\)`).
			WithArgs(1, uint64(0), uint64(100), 40000).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		mock.ExpectCommit()

		err := c.OverwriteStateByStartEndTime(&msg, &topic)
		assert.NoError(t, err)

		// Let's test an overwrite with state 50000 from 50 to 150, the result should be 0-50, 50-150, 150-200, 200-...
		msg = sharedStructs.StateOverwriteMessage{
			StartTimeUnixMs: 50,
			EndTimeUnixMs:   150,
			State:           50000,
		}

		mock.ExpectBeginTx(pgx.TxOptions{})
		// There is no state inbetween to be deleted, so we expect 0 deletes
		mock.ExpectExec(`DELETE FROM state
		WHERE asset_id = \$1
		AND start_time >= to_timestamp\(\$2\/1000\)
		AND start_time <= to_timestamp\(\$3\/1000\)
		`).WithArgs(1, uint64(50), uint64(150)).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))

		// The left update command will update the end time of the first state
		mock.ExpectExec(`UPDATE state
		SET end_time = to_timestamp\(\$2\/1000\)
		WHERE asset_id = \$1
		AND end_time > to_timestamp\(\$2\/1000\)
		AND end_time <= to_timestamp\(\$3\/1000\)
		`).WithArgs(1, uint64(50), uint64(150)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		// The right update command will update the start time of the second state
		mock.ExpectExec(`UPDATE state
		SET start_time = to_timestamp\(\$3\/1000\)
		WHERE asset_id = \$1
		AND start_time >= to_timestamp\(\$2\/1000\)
		AND start_time < to_timestamp\(\$3\/1000\)
		`).WithArgs(1, uint64(50), uint64(150)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		// The insert command will insert the new state
		mock.ExpectExec(`INSERT INTO state\(asset_id, start_time, end_time, state\)
		VALUES \(\$1, to_timestamp\(\$2\/1000\), to_timestamp\(\$3\/1000\), \$4\)`).
			WithArgs(1, uint64(50), uint64(150), 50000).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		mock.ExpectCommit()

		err = c.OverwriteStateByStartEndTime(&msg, &topic)
		assert.NoError(t, err)

	})
}
