package postgresql

import (
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"go.uber.org/zap"
	"time"
)

func (c *Connection) InsertHistorianValue(value []sharedStructs.HistorianValue, timestampMs int64, origin string, topic *sharedStructs.TopicDetails) error {
	assetId, err := c.GetOrInsertAsset(topic)
	if err != nil {
		return err
	}
	seconds := timestampMs / 1000
	nanoseconds := (timestampMs % 1000) * 1000000
	timestamp := time.Unix(seconds, nanoseconds)

	for _, v := range value {
		if v.IsNumeric {
			c.NumericalValuesChannel <- sharedStructs.DBHistorianValue{
				Timestamp: timestamp,
				Origin:    origin,
				AssetId:   int(assetId), // AssetId is passed around as uint64 preventing parsing errors, but is also an int in the database
				Value:     v,
			}
			c.numericalReceived.Add(1)
		} else {
			c.StringValuesChannel <- sharedStructs.DBHistorianValue{
				Timestamp: timestamp,
				Origin:    origin,
				AssetId:   int(assetId),
				Value:     v,
			}
			c.stringsReceived.Add(1)
		}
	}

	return nil
}

// Source This implementation is not thread-safe !
type Source struct {
	datachan  chan sharedStructs.DBHistorianValue
	isNumeric bool
}

func (s *Source) Next() bool {
	return len(s.datachan) != 0
}

func (s *Source) Values() ([]any, error) {
	select {
	case msg := <-s.datachan:
		values := make([]any, 5)
		values[0] = msg.Timestamp
		values[1] = msg.Value.Name
		values[2] = msg.Origin
		values[3] = msg.AssetId
		if s.isNumeric {
			values[4] = msg.GetValue().(float32)
		} else {
			values[4] = msg.GetValue().(string)
		}
		zap.S().Debugf("Values: %+v", values)
		return values, nil
	default:
		return nil, errors.New("no more rows available")
	}
}

func (s *Source) Err() error {
	return nil
}

func (c *Connection) tagWorker(tableName string, source *Source) {
	zap.S().Debugf("Starting tagWorker for %s", tableName)
	// The context is only used for preparation, not execution!

	var err error
	tableNameTemp := fmt.Sprintf("tmp_%s", tableName)
	var copiedIn int64

	for {
		sleepTime := c.calculateSleepTime(copiedIn)
		time.Sleep(sleepTime)

		if !source.Next() {
			zap.S().Debugf("No data available for %s", tableName)
			continue
		}

		txnExecutionCtx, txnExecutionCancel := get1MinuteContext()
		// Create transaction
		var txn pgx.Tx
		txn, err = c.Db.Begin(txnExecutionCtx)
		if err != nil {
			zap.S().Errorf("Failed to create transaction: %s (%s)", err, tableName)

			txnExecutionCancel()
			continue
		}

		// Create the temp table for COPY
		_, err = txn.Exec(txnExecutionCtx, fmt.Sprintf(`
	   CREATE TEMP TABLE %s
	          ( LIKE %s INCLUDING DEFAULTS )
	          ON COMMIT DROP;
	   `, tableNameTemp, tableName))
		if err != nil {
			zap.S().Errorf("Failed to execute statementCreateTmpTag: %s (%s)", err, tableName)
			rollbackCtx, rollbackCtxCncl := get5SecondContext()
			err = txn.Rollback(rollbackCtx)
			rollbackCtxCncl()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}

			txnExecutionCancel()
			continue
		}
		copiedIn, err = txn.CopyFrom(txnExecutionCtx, pgx.Identifier{tableNameTemp}, []string{
			"timestamp", "name", "origin", "asset_id", "value",
		}, source)

		if err != nil {
			zap.S().Warnf("Failed to execute stmtCopyToTag: %s (%s)", err, tableName)
			rollbackCtx, rollbackCtxCncl := get5SecondContext()
			err = txn.Rollback(rollbackCtx)
			rollbackCtxCncl()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			txnExecutionCancel()
			continue
		}

		// Do insert via statementInsertSelect
		_, err = txn.Exec(txnExecutionCtx, fmt.Sprintf(`
				INSERT INTO %s (SELECT * FROM %s) ON CONFLICT DO NOTHING;
			`, tableName, tableNameTemp))

		if err != nil {
			zap.S().Warnf("Failed to execute stmtCopyToTag: %s (%s)", err, tableName)
			rollbackCtx, rollbackCtxCncl := get5SecondContext()
			err = txn.Rollback(rollbackCtx)
			rollbackCtxCncl()
			if err != nil {
				zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			}
			txnExecutionCancel()
			continue
		}

		zap.S().Debugf("Pre-commit")
		now := time.Now()
		err = txn.Commit(txnExecutionCtx)
		c.commits.Add(1)
		c.commitTime.Add(uint64(time.Since(now).Milliseconds()))
		zap.S().Debugf("Committing to postgresql took: %s", time.Since(now))

		if err != nil {
			zap.S().Errorf("Failed to rollback transaction: %s (%s)", err, tableName)
			txnExecutionCancel()
			continue
		}
		zap.S().Debugf("Inserted %d values inside the %s table", copiedIn, tableName)
		c.databaseInserted.Add(uint64(copiedIn))
		txnExecutionCancel()
	}
}
