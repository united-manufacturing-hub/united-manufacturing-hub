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

	// If there is already a previous state, set it's end time to the new state's start time
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		UPDATE state
		SET    end_time = to_timestamp($2 / 1000.0)
		WHERE  asset_id = $1
			   AND end_time IS NULL
			   AND start_time < to_timestamp($2 / 1000.0) 
	`, int(assetId), msg.StartTimeUnixMs)

	if err != nil {
		zap.S().Warnf("Error updating previous state: %v (start: %v | state: %v) [%s]", err, msg.StartTimeUnixMs, msg.State, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	// Insert state
	// TODO: Error if there is already a state with the same start time
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
                        to_timestamp($2/1000.0),
                        $3
            )
		on conflict
		ON CONSTRAINT state_start_asset_uniq do nothing
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
	// This will do the following
	// 1. Delete all states for the asset between the start and end time
	// 2. Check if a state is reaching into the time range (from the left or right) and modify it's start/end time to be non-overlapping
	// 3. Insert the new state

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

	// Delete states between start and end time (inclusive) for the asset
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		DELETE FROM state
		WHERE  asset_id = $1
			   AND start_time >= to_timestamp($2 / 1000.0)
			   AND start_time <= to_timestamp($3 / 1000.0) 
	`, int(assetId), msg.StartTimeUnixMs, msg.EndTimeUnixMs)

	if err != nil {
		zap.S().Warnf("Error deleting states: %v (start: %v | end: %v) [%v]", err, msg.StartTimeUnixMs, msg.EndTimeUnixMs, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	// Check for overlapping state and modify it's end time
	cmdTag, err = tx.Exec(ctx, `
		UPDATE state
		SET    end_time = to_timestamp($2 / 1000.0)
		WHERE  asset_id = $1
			   AND end_time > to_timestamp($2 / 1000.0)
			   AND end_time <= to_timestamp($3 / 1000.0) 
	`, int(assetId), msg.StartTimeUnixMs, msg.EndTimeUnixMs)

	if err != nil {
		zap.S().Warnf("Error updating states (left): %v (start: %v | end: %v) [%v]", err, msg.StartTimeUnixMs, msg.EndTimeUnixMs, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	// Check for overlapping state and modify it's start time
	cmdTag, err = tx.Exec(ctx, `
		UPDATE state
		SET    start_time = to_timestamp($3 / 1000.0)
		WHERE  asset_id = $1
			   AND start_time >= to_timestamp($2 / 1000.0)
			   AND start_time < to_timestamp($3 / 1000.0) 
	`, int(assetId), msg.StartTimeUnixMs, msg.EndTimeUnixMs)

	if err != nil {
		zap.S().Warnf("Error updating states (right): %v (start: %v | end: %v) [%v]", err, msg.StartTimeUnixMs, msg.EndTimeUnixMs, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	// Insert state

	cmdTag, err = tx.Exec(ctx, `
		INSERT INTO state
            (asset_id,
             start_time,
             end_time,
             state)
		VALUES      ($1,
					 to_timestamp($2 / 1000.0),
					 to_timestamp($3 / 1000.0),
					 $4) 
	`, int(assetId), msg.StartTimeUnixMs, msg.EndTimeUnixMs, int(msg.State))

	if err != nil {
		zap.S().Warnf("Error inserting state: %v (start: %v | end: %v | state: %v) [%v]", err, msg.StartTimeUnixMs, msg.EndTimeUnixMs, msg.State, cmdTag)
		zap.S().Debugf("Message: %v (Topic: %v)", msg, topic)
		errR := tx.Rollback(ctx)
		if errR != nil {
			zap.S().Errorf("Error rolling back transaction: %v", errR)
		}
		return err
	}

	return tx.Commit(ctx)
}
