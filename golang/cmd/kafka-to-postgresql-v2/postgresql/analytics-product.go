package postgresql

import (
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
)

func (c *Connection) InsertProductAdd(msg *sharedStructs.ProductAddMessage, topic *sharedStructs.TopicDetails) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	productTypeId, err := c.GetOrInsertProductType(assetId, msg.ExternalProductId, 0)
	if err != nil {
		return err
	}

	// Start tx (this shouln't take more then 1 minute)
	ctx, cncl := get1MinuteContext()
	defer cncl()
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return err
	}
	// Insert product
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		INSERT INTO products (externalProductTypeId, productBatchId, assetId, startTime, endTime, quantity, badQuantity)
		VALUES ($1, $2, $3, to_timestamp($4/1000), to_timestamp($5/1000), $6, $7)
	`, int(productTypeId), msg.ProductBatchId, int(assetId), msg.StartTimeUnixMs, msg.EndTimeUnixMs, int(msg.Quantity), int(msg.BadQuantity))
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
	productTypeId, err := c.GetOrInsertProductType(assetId, msg.ExternalProductId, 0)
	if err != nil {
		return err
	}

	// Start tx
	ctx, cncl := get1MinuteContext()
	defer cncl()
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return err
	}

	// Update bad quantity with check integrated in WHERE clause
	cmdTag, err := tx.Exec(ctx, `
        UPDATE products
        SET badQuantity = badQuantity + $1
        WHERE externalProductTypeId = $2
          AND assetId = $3
          AND endTime = $4
          AND (quantity - badQuantity) >= $1
    `, int(msg.BadQuantity), productTypeId, assetId, msg.EndTimeUnixMs)

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
