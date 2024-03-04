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

	/*
		-- State Table
		-- Records the state changes of assets over time. State tracking supports ISA-95's goal of detailed monitoring and control of manufacturing operations.
		-- State Table
		CREATE TABLE states (
		    stateId INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		    assetId INTEGER REFERENCES assets(id),
		    startTime TIMESTAMPTZ NOT NULL,
			endTime TIMESTAMPTZ,
		    state INT NOT NULL,
		    CHECK (state >= 0),
		    CONSTRAINT state_start_asset_uniq UNIQUE (startTime, assetId)
		);
		-- creating hypertable
		SELECT create_hypertable('states', 'startTime');

		-- creating an index to increase performance
		CREATE INDEX idx_states_asset_starttime ON states(assetId, startTime DESC);
	*/

	// Start tx (this shouln't take more then 1 minute)
	ctx, cncl := get1MinuteContext()
	defer cncl()
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return err
	}

	// If there is already a previous state, set it's end time to the new state's start time
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		UPDATE states
		SET endTime = to_timestamp($2/1000)
		WHERE assetId = $1
		AND endTime IS NULL
		AND startTime < to_timestamp($2/1000)
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
	cmdTag, err = tx.Exec(ctx, `
		INSERT INTO states (assetId, startTime, state)
		VALUES ($1, to_timestamp($2/1000), $3)
		ON CONFLICT ON CONSTRAINT state_start_asset_uniq
		DO NOTHING
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
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return err
	}

	// Delete states between start and end time (inclusive) for the asset
	var cmdTag pgconn.CommandTag
	cmdTag, err = tx.Exec(ctx, `
		DELETE FROM states
		WHERE assetId = $1
		AND startTime >= to_timestamp($2/1000)
		AND startTime <= to_timestamp($3/1000)
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
		UPDATE states
		SET endTime = to_timestamp($2/1000)
		WHERE assetId = $1
		AND endTime > to_timestamp($2/1000)
		AND endTime <= to_timestamp($3/1000)
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
		UPDATE states
		SET startTime = to_timestamp($3/1000)
		WHERE assetId = $1
		AND startTime >= to_timestamp($2/1000)
		AND startTime < to_timestamp($3/1000)
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
		INSERT INTO states (assetId, startTime, endTime, state)
		VALUES ($1, to_timestamp($2/1000), to_timestamp($3/1000), $4)
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
