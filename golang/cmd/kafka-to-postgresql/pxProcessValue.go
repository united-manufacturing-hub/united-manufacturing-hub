package main

import (
	"context"
	"database/sql"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"github.com/lib/pq"
	"go.uber.org/zap"
	"time"
)

type ProcessValue struct{}

// Contains timestamp_ms and 1 other key, which is a float64 or int64
type processValue map[string]interface{}

var processValueChannel chan *kafka.Message

// startProcessValueChannel reads messages from the processValueChannel and inserts them into a temporary buffer, before committing them to the database
func startProcessValueQueueAggregator() {
	processValueChannel = make(chan *kafka.Message, 1000)

	messages := make([]*kafka.Message, 1000)
	writeToDbTimer := time.NewTicker(time.Second * 5)

	for !ShuttingDown {
		select {
		case msg := <-processValueChannel:
			{
				messages = append(messages, msg)
				break
			}
		case <-writeToDbTimer.C:
			{
				zap.S().Debugf("[HT][PV] Messages length: %d", len(messages))
				if len(messages) == 0 {
					writeToDbTimer.Reset(time.Second * 5)
					continue
				}
				putBackMsg, err, putback, reason := writeProcessValueToDatabase(messages)
				if putback {
					for _, message := range putBackMsg {
						errStr := err.Error()
						highThroughputPutBackChannel <- PutBackChanMsg{
							msg:         message,
							reason:      reason,
							errorString: &errStr,
						}
					}
				}
				break
			}
		}
	}
	for _, message := range messages {
		highThroughputPutBackChannel <- PutBackChanMsg{
			msg:         message,
			reason:      "Shutting down",
			errorString: nil,
		}
	}
}

func writeProcessValueToDatabase(messages []*kafka.Message) (putBackMsg []*kafka.Message, err error, putback bool, reason string) {
	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnCtxCl()
	var txn *sql.Tx = nil
	txn, err = db.BeginTx(txnCtx, nil)
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return messages, err, true, "Error starting transaction"
	}

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnStmtCtxCl()
	stmt := txn.StmtContext(txnStmtCtx, statement.CreateTmpProcessValueTable64)

	_, err = stmt.Exec()
	if err != nil {
		zap.S().Errorf("Error creating temporary table: %s", err.Error())
		return messages, err, true, "Error creating temporary table"
	}

	putBackMsg = make([]*kafka.Message, 0)

	txnStmtCopyCtx, txnStmtCopyCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnStmtCopyCtxCl()
	var stmtCopy *sql.Stmt
	stmtCopy, err = txn.PrepareContext(txnStmtCopyCtx, pq.CopyIn("tmp_processvaluetable64", "timestamp", "asset_id", "value", "valuename"))
	defer stmtCopy.Close()
	if err != nil {
		zap.S().Errorf("Error preparing copy statement: %s", err.Error())
		return messages, err, true, "Error preparing copy statement"
	}

	// Copy into the temporary table
	for _, message := range messages {
		couldParse, parsedMessage := ParseMessage(message)
		if !couldParse {
			continue
		}

		var sC processValue
		err = jsoniter.Unmarshal(parsedMessage.Payload, &sC)
		if err != nil {
			continue
		}
		AssetTableID, success := GetAssetTableID(parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId)
		if !success {
			zap.S().Errorf("Error getting asset table id: %s for %s %s %s", err.Error(), parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId)
			putBackMsg = append(putBackMsg, message)
			continue
		}

		if timestampString, timestampInParsedMessagePayload := sC["timestamp_ms"]; timestampInParsedMessagePayload {
			timestampMs, timestampIsUint64 := timestampString.(uint64)
			if !timestampIsUint64 {
				// Timestamp is malformed, drop message
				continue
			}
			for k, v := range sC {
				switch k {
				case "timestamp_ms":
				// Copied these exceptions from mqtt-to-postgresql
				case "measurement":
				case "serial_number":
					break
				default:
					value, valueIsFloat64 := v.(float64)
					if !valueIsFloat64 {
						// Value is malformed, skip to next key
						continue
					}

					timestamp := time.Unix(0, int64(timestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")
					txnStmtCopyExecCtx, txnStmtCopyExecCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
					_, err = stmtCopy.ExecContext(txnStmtCopyExecCtx, timestamp, AssetTableID, k, value)
					defer txnStmtCopyExecCtxCl()
					if err != nil {
						zap.S().Errorf("Error inserting into temporary table: %s", err.Error())
						return messages, err, true, "Error inserting into temporary table"
					}
				}
			}
		}
	}

	txnStmtCopyToPVTCtx, txnStmtCopyToPVTCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnStmtCopyToPVTCtxCl()
	var stmtCopyToPVT *sql.Stmt
	stmtCopyToPVT, err = txn.PrepareContext(txnStmtCopyToPVTCtx, `
			INSERT INTO processvaluetable (SELECT * FROM tmp_processvaluetable64) ON CONFLICT DO NOTHING;
		`)
	if err != nil {
		zap.S().Errorf("Error preparing copy to process value table statement: %s", err.Error())
		return messages, err, true, "Error preparing copy to process value table statement"
	}

	txnStmtCopyToPVTExecCtx, txnStmtCopyToPVTExecCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnStmtCopyToPVTExecCtxCl()
	_, err = stmtCopyToPVT.ExecContext(txnStmtCopyToPVTExecCtx)
	if err != nil {
		zap.S().Errorf("Error copying to process value table: %s", err.Error())
		return messages, err, true, "Error copying to process value table"
	}
	return putBackMsg, nil, true, "Error executing insertion process"
}
