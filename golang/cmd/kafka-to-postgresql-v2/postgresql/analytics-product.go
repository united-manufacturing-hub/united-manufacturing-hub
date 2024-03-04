package postgresql

import (
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
	`, int(productTypeId), msg.ProductBatchId, int(assetId), msg.StartTime, msg.EndTime, int(msg.Quantity), int(msg.BadQuantity))
	if err != nil {
		zap.S().Warnf("Error inserting work order: %v (productTypeId: %v) [%s]", err, productTypeId, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
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

	// Get product quantity by productTypeId for endTime
	var quantity int
	var badQuantity int
	err = tx.QueryRow(ctx, `
		SELECT quantity, badQuantity
		FROM products
		WHERE externalProductTypeId = $1 AND assetId = $2 AND endTime = $3
	`, productTypeId, assetId, msg.EndTime).Scan(&quantity, &badQuantity)
	if err != nil {
		zap.S().Warnf("Error getting product quantity: %v", err)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	// Check if (quantity - badQuantity) >= msg.BadQuantity, otherwise error
	newBadQuantity := uint64(badQuantity) + msg.BadQuantity

	if uint64(quantity) < newBadQuantity {
		zap.S().Warnf("Bad quantity is greater than available quantity: %v", msg)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return nil
	}

	// Update bad quantity
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		UPDATE products
		SET badQuantity = $1
		WHERE externalProductTypeId = $2 AND assetId = $3 AND endTime = $4
	`, int(newBadQuantity), productTypeId, assetId, msg.EndTime)

	if err != nil {
		zap.S().Warnf("Error updating bad quantity: %v [%s]", err, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
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
