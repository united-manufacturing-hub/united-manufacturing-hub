package main

import (
	"database/sql"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"github.com/lib/pq"
	"go.uber.org/zap"
	"time"
)

// Contains timestamp_ms and 1 other key, which is a float64 or int64
type processValue map[string]interface{}

var processValueChannel chan *kafka.Message

// startProcessValueChannel reads messages from the processValueChannel and inserts them into a temporary buffer, before committing them to the database
func startProcessValueQueueAggregator() {
	// This channel is used to aggregate messages from the kafka queue, for further processing
	// It size was chosen, to prevent timescaledb from choking on large inserts
	processValueChannel = make(chan *kafka.Message, 5000)

	messages := make([]*kafka.Message, 0)
	writeToDbTimer := time.NewTicker(time.Second * 5)

	for !ShuttingDown {
		select {
		case msg := <-processValueChannel:
			{
				messages = append(messages, msg)
				// This checks for >= 5000, because we don't want to block the channel (see size of the processValueChannel)
				if len(messages) >= 5000 {
					//zap.S().Debugf("[HT][PV][AA] Messages length: %d", len(messages))
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
					messages = make([]*kafka.Message, 0)
					continue
				}
				break
			}
		case <-writeToDbTimer.C:
			{
				//zap.S().Debugf("[HT][PV] Messages length: %d", len(messages))
				if len(messages) == 0 {
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
				messages = make([]*kafka.Message, 0)

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
	//zap.S().Debugf("[HT][PV] Writing %d messages to database", len(messages))
	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return messages, err, true, "Error starting transaction"
	}

	//zap.S().Debugf("[HT][PV] 1")
	{
		stmt := txn.Stmt(statement.CreateTmpProcessValueTable64)

		_, err = stmt.Exec()
		if err != nil {
			txn.Rollback()
			zap.S().Errorf("Error creating temporary table: %s", err.Error())
			return messages, err, true, "Error creating temporary table"
		}
	}
	putBackMsg = make([]*kafka.Message, 0)
	// toCommit is used for stats only, it just increments, whenever a message was added to the transaction.
	// at the end, this count is added to the global Commit counter
	toCommit := float64(0)
	//zap.S().Debugf("[HT][PV] 2")
	{

		var stmtCopy *sql.Stmt
		stmtCopy, err = txn.Prepare(pq.CopyIn("tmp_processvaluetable64", "timestamp", "asset_id", "value", "valuename"))
		if err != nil {
			zap.S().Errorf("Error preparing copy statement: %s", err.Error())
			return messages, err, true, "Error preparing copy statement"
		}

		//zap.S().Debugf("[HT][PV] 3 %d", len(messages))
		// Copy into the temporary table
		for _, message := range messages {
			couldParse, parsedMessage := ParseMessage(message)
			if !couldParse {
				//zap.S().Debugf("[HT][PV] Could not parse message: %s", message.String())

				continue
			}

			// sC is the payload, parsed as processValue
			var sC processValue
			err = jsoniter.Unmarshal(parsedMessage.Payload, &sC)
			if err != nil {

				//zap.S().Debugf("[HT][PV] Could not parse message: %s", err.Error())
				continue
			}
			AssetTableID, success := GetAssetTableID(parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId)
			if !success {
				zap.S().Errorf("Error getting asset table id: %s for %s %s %s", err.Error(), parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId)
				putBackMsg = append(putBackMsg, message)
				continue
			}
			timestampString, timestampInParsedMessagePayload := sC["timestamp_ms"]
			if !timestampInParsedMessagePayload {

				//zap.S().Debugf("[HT][PV] Timestamp not in parsed message payload")
				continue
			}
			var tsF64 float64
			tsF64, err = getFloat(timestampString)
			if err != nil {
				//zap.S().Debugf("[HT][PV] Could not parse timestamp: %s", err.Error())
				continue
			}

			timestampMs := uint64(tsF64)
			////zap.S().Debugf("[HT][PV] Timestamp: %d", timestampMs)
			for k, v := range sC {
				switch k {
				case "timestamp_ms":
				// Copied these exceptions from mqtt-to-postgresql
				// These are here for historical reasons
				case "measurement":
				case "serial_number":
					break
				default:
					////zap.S().Debugf("[HT][PV] Inserting %s: %v", k, v)
					value, valueIsFloat64 := v.(float64)
					if !valueIsFloat64 {

						////zap.S().Debugf("[HT][PV] Value is not a float64")
						// Value is malformed, skip to next key
						continue
					}

					// This coversion is necessary for postgres
					timestamp := time.Unix(0, int64(timestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")

					////zap.S().Debugf("[HT][PV] Inserting %d, %s, %f, %s", AssetTableID, timestamp, value, k)
					_, err = stmtCopy.Exec(timestamp, AssetTableID, value, k)
					if err != nil {
						zap.S().Errorf("Error inserting into temporary table: %s", err.Error())
						txn.Rollback()
						return messages, err, true, "Error inserting into temporary table"
					}
					toCommit += 1
				}
			}
		}
		//zap.S().Debugf("Pre copy closed")
		err = stmtCopy.Close()
		//zap.S().Debugf("Post copy closed")
		if err != nil {
			txn.Rollback()
			return messages, err, true, "Failed to close copy statement"
		}
	}
	//zap.S().Debugf("pre insert prepare")
	var stmtCopyToPVT *sql.Stmt
	stmtCopyToPVT, err = txn.Prepare(`
			INSERT INTO processvaluetable (SELECT * FROM tmp_processvaluetable64) ON CONFLICT DO NOTHING;
		`)
	//zap.S().Debugf("post insert prepare")
	if err != nil {
		txn.Rollback()
		zap.S().Errorf("Error preparing copy to process value table statement: %s", err.Error())
		return messages, err, true, "Error preparing copy to process value table statement"
	}

	//zap.S().Debugf("pre exec")
	_, err = stmtCopyToPVT.Exec()
	//zap.S().Debugf("post exec")
	if err != nil {
		txn.Rollback()
		zap.S().Errorf("Error copying to process value table: %s", err.Error())
		return messages, err, true, "Error copying to process value table"
	}

	//zap.S().Debugf("pre close")
	err = stmtCopyToPVT.Close()
	if err != nil {
		txn.Rollback()
		zap.S().Errorf("Error closing stmtCopytoPVT: %s", err.Error())
		return messages, err, true, "Error closing stmtCopytoPVT"
	}
	//zap.S().Debugf("post close")

	if isDryRun {
		err = txn.Rollback()
		if err != nil {
			return messages, err, true, "Failed to rollback"
		}
		return putBackMsg, nil, true, "AssetID not found"
	} else {
		//zap.S().Debugf("Pre-commit to process value table: %d", toCommit)
		err = txn.Commit()
		if err != nil {
			return messages, err, true, "Failed to commit"
		}
		//zap.S().Debugf("Committed %d messages, putting back %d messages", len(messages)-len(putBackMsg), len(putBackMsg))
		if len(putBackMsg) > 0 {
			return putBackMsg, nil, true, "AssetID not found"
		}
		PutBacks += float64(len(putBackMsg))
		Commits += toCommit
	}
	return putBackMsg, nil, false, ""
}
