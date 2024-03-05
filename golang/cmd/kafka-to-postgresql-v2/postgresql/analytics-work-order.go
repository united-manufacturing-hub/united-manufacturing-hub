package postgresql

import (
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/helper"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
)

func (c *Connection) InsertWorkOrderCreate(msg *sharedStructs.WorkOrderCreateMessage, topic *sharedStructs.TopicDetails) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	productTypeId, err := c.GetOrInsertProductType(assetId, msg.Product.ExternalProductId, msg.Product.CycleTimeMs)
	if err != nil {
		return err
	}
	// Start tx (this shouldn't take more then 1 minute)
	ctx, cncl := get1MinuteContext()
	defer cncl()
	tx, err := c.Db.Begin(ctx)
	if err != nil {
		return err
	}
	// Don't forget to convert unix ms to timestamptz
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		INSERT INTO work_order(external_work_order_id, asset_id, product_type_id, quantity, status, start_time, end_time)
		VALUES ($1, $2, $3, $4, $5, CASE WHEN $6 IS NOT NULL THEN to_timestamp($6/1000) END, CASE WHEN $7 IS NOT NULL THEN to_timestamp($7/1000) END)
	`, msg.ExternalWorkOrderId, int(assetId), int(productTypeId), int(msg.Quantity), int(msg.Status), helper.Uint64PtrToInt64Ptr(msg.StartTimeUnixMs), helper.Uint64PtrToInt64Ptr(msg.EndTimeUnixMs))
	if err != nil {
		zap.S().Warnf("Error inserting work order: %v (workOrderId: %v) [%s]", err, msg.ExternalWorkOrderId, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}
	return tx.Commit(ctx)
}

func (c *Connection) UpdateWorkOrderSetStart(msg *sharedStructs.WorkOrderStartMessage, topic *sharedStructs.TopicDetails) error {
	// Update work-order by external_work_order_id
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}

	// Start tx (this shouldn't take more then 1 minute)
	ctx, cncl := get1MinuteContext()
	defer cncl()
	tx, err := c.Db.Begin(ctx)
	if err != nil {
		return err
	}

	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		UPDATE work_order
		SET status = 1, start_time = to_timestamp($2 / 1000)
		WHERE external_work_order_id = $1
		  AND status = 0 
		  AND start_time IS NULL
		  AND asset_id = $3
	`, msg.ExternalWorkOrderId, msg.StartTimeUnixMs, int(assetId))
	if err != nil {
		zap.S().Warnf("Error updating work order: %v (workOrderId: %v) [%s]", err, msg.ExternalWorkOrderId, cmdTag)
		zap.S().Debugf("Message: %v", msg)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (c *Connection) UpdateWorkOrderSetStop(msg *sharedStructs.WorkOrderStopMessage, topic *sharedStructs.TopicDetails) error {
	// Update work-order by external_work_order_id
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	// Start tx (this shouldn't take more then 1 minute)
	ctx, cncl := get1MinuteContext()
	defer cncl()
	tx, err := c.Db.Begin(ctx)
	if err != nil {
		return err
	}

	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		UPDATE work_order
		SET status = 2, end_time = to_timestamp($2 / 1000)
		WHERE external_work_order_id = $1
		  AND status = 1
		  AND end_time IS NULL
		  AND asset_id = $3
	`, msg.ExternalWorkOrderId, msg.EndTimeUnixMs, int(assetId))
	if err != nil {
		zap.S().Warnf("Error updating work order: %v (workOrderId: %v) [%s]", err, msg.ExternalWorkOrderId, cmdTag)
		zap.S().Debugf("Message: %v", msg)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return err
	}

	return nil
}
