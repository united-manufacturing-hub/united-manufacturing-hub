package postgresql

import (
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"testing"
)

func TestInsertWorkOrder(t *testing.T) {
	c := CreateMockConnection(t)
	defer c.db.Close()

	// Cast c.db to pgxmock to access the underlying mock
	mock, ok := c.db.(pgxmock.PgxPoolIface)
	assert.True(t, ok)

	t.Run("insert", func(t *testing.T) {
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
		INSERT INTO work_orders \(externalWorkOrderId, assetId, productTypeId, quantity, status, to_timestamp\(\$6 \/ 1000\), to_timestamp\(\$7 \/ 1000\)\)
		VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7\)
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

		// Expect Exec from InsertWorkOrderStart
		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`
		UPDATE work_orders
		SET status = 1, startTime = to_timestamp\(\$2 \/ 1000\)
		WHERE externalWorkOrderId = \$1
	`).WithArgs("#1274", uint64(0)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit()

		err := c.InsertWorkOrderStart(&msg)
		assert.NoError(t, err)
	})

	t.Run("stop", func(t *testing.T) {
		msg := sharedStructs.WorkOrderStopMessage{
			ExternalWorkOrderId: "#1274",
			EndTimeUnixMs:       0,
		}

		// Expect Exec from InsertWorkOrderStop
		mock.ExpectBeginTx(pgx.TxOptions{})
		mock.ExpectExec(`
		UPDATE work_orders
		SET status = 2, endTime = to_timestamp\(\$2 \/ 1000\)
		WHERE externalWorkOrderId = \$1
		`).WithArgs("#1274", uint64(0)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit()

		err := c.InsertWorkOrderStop(&msg)
		assert.NoError(t, err)

	})
}
