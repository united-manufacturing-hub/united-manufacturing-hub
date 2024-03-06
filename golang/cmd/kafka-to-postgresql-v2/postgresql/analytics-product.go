package postgresql

import (
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/helper"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
)

func (c *Connection) InsertProductAdd(msg *sharedStructs.ProductAddMessage, topic *sharedStructs.TopicDetails) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	productTypeId, err := c.GetOrInsertProductType(assetId, msg.ExternalProductId, nil)
	if err != nil {
		return err
	}

	// Start tx (this shouln't take more then 1 minute)
	ctx, cncl := get1MinuteContext()
	defer cncl()
	tx, err := c.Db.Begin(ctx)
	if err != nil {
		return err
	}
	// Insert producth
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
			INSERT INTO product
            (
                        external_product_type_id,
                        product_batch_id,
                        asset_id,
                        start_time,
                        end_time,
                        quantity,
                        bad_quantity
            )
            VALUES
            	(
                        $1,
                        $2::TEXT,
                        $3,
                        CASE
							WHEN $4::BIGINT IS NOT NULL THEN to_timestamp($4::BIGINT  / 1000.0)
					   		ELSE NULL
                        END::timestamptz,
                        to_timestamp($5::BIGINT  / 1000.0),
                        $6,
                        $7::int
				)
		`, int(productTypeId), helper.StringPtrToNullString(msg.ProductBatchId), int(assetId), helper.Uint64PtrToNullInt64(msg.StartTimeUnixMs), msg.EndTimeUnixMs, int(msg.Quantity), helper.Uint64PtrToNullInt64(msg.BadQuantity))
	if err != nil {
		zap.S().Warnf("Error inserting product: %v (productTypeId: %v) [%s]", err, productTypeId, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}
	return tx.Commit(ctx)
}

func (c *Connection) UpdateBadQuantityForProduct(msg *sharedStructs.ProductSetBadQuantityMessage, topic *sharedStructs.TopicDetails) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	productTypeId, err := c.GetOrInsertProductType(assetId, msg.ExternalProductId, nil)
	if err != nil {
		return err
	}

	// Start tx
	ctx, cncl := get1MinuteContext()
	defer cncl()
	tx, err := c.Db.Begin(ctx)
	if err != nil {
		return err
	}

	// Update bad quantity with check integrated in WHERE clause
	cmdTag, err := tx.Exec(ctx, `
        UPDATE product
		SET    bad_quantity = bad_quantity + $1
		WHERE  external_product_type_id = $2
			   AND asset_id = $3
			   AND end_time = to_timestamp($4::BIGINT / 1000.0)
			   AND ( quantity - bad_quantity ) >= $1 
    `, int(msg.BadQuantity), int(productTypeId), int(assetId), msg.EndTimeUnixMs)

	if err != nil {
		zap.S().Warnf("Error updating bad quantity: %v", err)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	// Check if the update operation affected any rows
	if cmdTag.RowsAffected() == 0 {
		zap.S().Warnf("Bad quantity update condition not met for product: %v", msg)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction due to condition not met: %v", errR)
		}
		return fmt.Errorf("bad quantity update condition not met for product: %v", msg)
	}

	return tx.Commit(ctx)
}
