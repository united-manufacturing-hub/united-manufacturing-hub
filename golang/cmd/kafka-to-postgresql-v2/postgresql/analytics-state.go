package postgresql

import (
	"github.com/jackc/pgx/v5/pgconn"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
)

func (c *Connection) InsertStateAdd(msg *sharedStructs.StateAddMessage, topic *sharedStructs.TopicDetails) error {
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

	// Insert state
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		INSERT INTO state
            (
                        asset_id,
                        start_time,
                        state
            )
            VALUES
            (
                        $1,
                        to_timestamp($2::BIGINT  / 1000.0),
                        $3
            )
	`, int(assetId), msg.StartTimeUnixMs, int(msg.State))

	if err != nil {
		zap.S().Warnf("Error inserting state: %v (start: %v | state: %v) [%s]", err, msg.StartTimeUnixMs, msg.State, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	return tx.Commit(ctx)
}

func (c *Connection) OverwriteStateByStartEndTime(msg *sharedStructs.StateOverwriteMessage, topic *sharedStructs.TopicDetails) error {
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

	// 3 step process
	// First we INSERT a new state based on the next state after ours
	// Then we DELETE the states that are between the start and end time
	// Finally we insert our new state

	// Insert state
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
INSERT INTO state (asset_id, start_time, state)
SELECT $1, $3, state
FROM state s
WHERE s.asset_id = $1
				   AND s.start_time > to_timestamp($2::BIGINT  / 1000.0)
                   AND s.start_time < to_timestamp($4::BIGINT  / 1000.0)
ORDER BY s.start_time DESC
LIMIT 1;
	`, int(assetId), msg.StartTimeUnixMs, msg.EndTimeUnixMs)

	if err != nil {
		zap.S().Warnf("Error inserting state: %v (start: %v | end: %v) [%s]", err, msg.StartTimeUnixMs, msg.EndTimeUnixMs, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	// Delete states
	cmdTag, err = tx.Exec(ctx, `
DELETE FROM state
    WHERE asset_id = $1
    AND start_time >= to_timestamp($2::BIGINT  / 1000.0)
    AND start_time < to_timestamp($3::BIGINT  / 1000.0)
    	`, int(assetId), msg.StartTimeUnixMs, msg.EndTimeUnixMs)

	if err != nil {
		zap.S().Warnf("Error deleting states: %v (start: %v | end: %v) [%s]", err, msg.StartTimeUnixMs, msg.EndTimeUnixMs, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
	}

	// Insert state
	cmdTag, err = tx.Exec(ctx, `
		INSERT INTO state
			(
						asset_id,
						start_time,
						state
			)
			VALUES
			(
						$1,
						to_timestamp($2::BIGINT  / 1000.0),
						$3
			)
	`, int(assetId), msg.StartTimeUnixMs, int(msg.State))
	if err != nil {
		zap.S().Warnf("Error inserting state: %v (start: %v | state: %v) [%s]", err, msg.StartTimeUnixMs, msg.State, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	return tx.Commit(ctx)
}
