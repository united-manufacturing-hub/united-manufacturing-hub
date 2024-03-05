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
	tx, err := c.Db.Begin(ctx)
	if err != nil {
		return err
	}

	// Insert product_type
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		INSERT INTO product_type
            (
                        external_product_type_id,
                        cycle_time_ms,
                        asset_id
            )
            VALUES
            (
                        $1,
                        $2,
                        $3
            )
			on conflict
				(
							external_product_type_id,
							asset_id
				)
				do nothing
	`, msg.ExternalProductTypeId, int(msg.CycleTimeMs), int(assetId))
	if err != nil {
		zap.S().Warnf("Error inserting product-type: %v (external_product_type_id: %v) [%s]", err, msg.ExternalProductTypeId, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}
	return tx.Commit(ctx)
}
