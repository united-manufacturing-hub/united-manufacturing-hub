package postgresql

import (
	"github.com/jackc/pgx/v5/pgconn"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
)

func (c *Connection) InsertProductTypeCreate(msg *sharedStructs.ProductTypeCreateMessage, topic *sharedStructs.TopicDetails) error {
	assetId, err := c.GetOrInsertAsset(topic)
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

	// Insert product_type
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		INSERT INTO product_types (externalProductTypeId, cycleTime, assetId)
		VALUES ($1, $2, $3)
		ON CONFLICT (externalProductTypeId, assetId) DO NOTHING
	`, msg.ExternalProductTypeId, int(msg.CycleTimeMs), int(assetId))
	if err != nil {
		zap.S().Warnf("Error inserting product-type: %v (externalProductTypeId: %v) [%s]", err, msg.ExternalProductTypeId, cmdTag)
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
