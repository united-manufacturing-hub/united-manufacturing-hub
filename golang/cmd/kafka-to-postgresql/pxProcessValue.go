package main

import (
	"database/sql"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"github.com/lib/pq"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"os"
	"strconv"
	"time"
)

// Contains timestamp_ms and 1 other key, which is a float64 or int64
type processValue map[string]interface{}

var processValueChannel chan *kafka.Message

// startProcessValueChannel reads messages from the processValueChannel and inserts them into a temporary buffer, before committing them to the database
func startProcessValueQueueAggregator() {
	chanSize := 500_000
	if os.Getenv("PV_CHANNEL_SIZE") != "" {
		atoi, err := strconv.Atoi(os.Getenv("PV_CHANNEL_SIZE"))
		if err != nil {
			zap.S().Warnf("[HT][PV] PV_CHANNEL_SIZE is not a valid integer: %s", err.Error())
		}
		chanSize = atoi
	}
	writeToDbTimer := time.NewTicker(time.Second * 5)
	if os.Getenv("PV_WRITE_TO_DB_INTERVAL") != "" {
		atoi, err := strconv.Atoi(os.Getenv("PV_WRITE_TO_DB_INTERVAL"))
		if err != nil {
			zap.S().Warnf("[HT][PV] PV_WRITE_TO_DB_INTERVAL is not a valid integer: %s", err.Error())
		}
		writeToDbTimer = time.NewTicker(time.Second * time.Duration(atoi))
	}

	// This channel is used to aggregate messages from the kafka queue, for further processing
	// It size was chosen, to prevent timescaledb from choking on large inserts
	processValueChannel = make(chan *kafka.Message, chanSize)
	messages := make([]*kafka.Message, 0)

	for !ShuttingDown {
		select {
		case msg := <-processValueChannel:
			{
				messages = append(messages, msg)
				// This checks for >= 500000, because we don't want to block the channel (see size of the processValueChannel)
				if len(messages) >= chanSize {
					//zap.S().Debugf("[HT][PV][AA] KafkaMessages length: %d", len(messages))
					putBackMsg, putback, reason, err := writeProcessValueToDatabase(messages)
					if putback {
						for _, message := range putBackMsg {
							var errStr string
							if err != nil {
								errStr = err.Error()
							}
							highThroughputPutBackChannel <- internal.PutBackChanMsg{
								Msg:         message,
								Reason:      reason,
								ErrorString: &errStr,
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
				//zap.S().Debugf("[HT][PV] KafkaMessages length: %d", len(messages))
				if len(messages) == 0 {
					continue
				}
				putBackMsg, putback, reason, err := writeProcessValueToDatabase(messages)
				if putback {
					for _, message := range putBackMsg {
						var errStr string
						if err != nil {
							errStr = err.Error()
						}
						highThroughputPutBackChannel <- internal.PutBackChanMsg{
							Msg:         message,
							Reason:      reason,
							ErrorString: &errStr,
						}
					}
				}
				messages = make([]*kafka.Message, 0)

				break
			}
		}
	}
	for _, message := range messages {
		highThroughputPutBackChannel <- internal.PutBackChanMsg{

			Msg:    message,
			Reason: "Shutting down",
		}
	}
}

func writeProcessValueToDatabase(messages []*kafka.Message) (
	putBackMsg []*kafka.Message,
	putback bool,
	reason string,
	err error) {
	zap.S().Debugf("[HT][PV] Writing %d messages to database", len(messages))
	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return messages, true, "Error starting transaction", err
	}

	zap.S().Debugf("[HT][PV] 1")
	{
		stmt := txn.Stmt(statement.CreateTmpProcessValueTable64)

		_, err = stmt.Exec()
		if err != nil {
			zap.S().Errorf("Error creating temporary table: %s", err.Error())
			errR := txn.Rollback()
			if errR != nil {
				zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
			}
			return messages, true, "Error creating temporary table", err
		}
	}
	putBackMsg = make([]*kafka.Message, 0)
	// toCommit is used for stats only, it just increments, whenever a message was added to the transaction.
	// at the end, this count is added to the global Commit counter
	toCommit := float64(0)
	zap.S().Debugf("[HT][PV] 2")
	{

		var stmtCopy *sql.Stmt
		stmtCopy, err = txn.Prepare(pq.CopyIn("tmp_processvaluetable64", "timestamp", "asset_id", "value", "valuename"))
		if err != nil {
			zap.S().Errorf("Error preparing copy statement: %s", err.Error())
			return messages, true, "Error preparing copy statement", err
		}

		zap.S().Debugf("[HT][PV] 3 %d", len(messages))
		// Copy into the temporary table
		for _, message := range messages {
			couldParse, parsedMessage := internal.ParseMessage(message)
			if !couldParse {
				zap.S().Errorf("[HT][PV] Could not parse message: %s", message.String())
				putBackMsg = append(putBackMsg, message)
				continue
			}

			// sC is the payload, parsed as processValue
			var sC processValue
			err = jsoniter.Unmarshal(parsedMessage.Payload, &sC)
			if err != nil {
				zap.S().Errorf("[HT][PV] Could not unmarshal message: %s", err.Error())
				putBackMsg = append(putBackMsg, message)
				continue
			}
			AssetTableID, success := GetAssetTableID(
				parsedMessage.CustomerId,
				parsedMessage.Location,
				parsedMessage.AssetId)
			if !success {
				zap.S().Errorf(
					"Error getting asset table id for %s %s %s",
					parsedMessage.CustomerId,
					parsedMessage.Location,
					parsedMessage.AssetId)
				putBackMsg = append(putBackMsg, message)
				continue
			}
			timestampString, timestampInParsedMessagePayload := sC["timestamp_ms"]
			if !timestampInParsedMessagePayload {

				zap.S().Debugf("[HT][PV] Timestamp not in parsed message payload")
				continue
			}
			var tsF64 float64
			tsF64, err = getFloat(timestampString)
			if err != nil {
				zap.S().Debugf("[HT][PV] Could not parse timestamp: %s", err.Error())
				continue
			}

			timestampMs := uint64(tsF64)
			//zap.S().Debugf("[HT][PV] Timestamp: %d", timestampMs)
			for k, v := range sC {
				switch k {
				case "timestamp_ms":
				// Copied these exceptions from mqtt-to-postgresql
				// These are here for historical reasons
				case "measurement":
				case "serial_number":
				default:
					//zap.S().Debugf("[HT][PV] Inserting %s: %v", k, v)
					value, valueIsFloat64 := v.(float64)
					if !valueIsFloat64 {

						//zap.S().Debugf("[HT][PV] Value is not a float64")
						// Value is malformed, skip to next key
						continue
					}

					// This coversion is necessary for postgres
					timestamp := time.Unix(0, int64(timestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")

					//zap.S().Debugf("[HT][PV] Inserting %d, %s, %f, %s", AssetTableID, timestamp, value, k)
					_, err = stmtCopy.Exec(timestamp, AssetTableID, value, k)
					if err != nil {
						zap.S().Errorf("Error inserting into temporary table: %s", err.Error())
						errR := txn.Rollback()
						if errR != nil {
							zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
						}
						return messages, true, "Error inserting into temporary table", err
					}
					toCommit += 1
				}
			}
		}
		zap.S().Debugf("Pre copy closed")
		err = stmtCopy.Close()
		zap.S().Debugf("Post copy closed")
		if err != nil {
			errR := txn.Rollback()
			if errR != nil {
				zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
			}
			return messages, true, "Failed to close copy statement", err
		}
	}
	zap.S().Debugf("pre insert prepare")
	var stmtCopyToPVT *sql.Stmt
	stmtCopyToPVT, err = txn.Prepare(
		`
			INSERT INTO processvaluetable (SELECT * FROM tmp_processvaluetable64) ON CONFLICT DO NOTHING;
		`)
	zap.S().Debugf("post insert prepare")
	if err != nil {
		errR := txn.Rollback()
		if errR != nil {
			zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
		}
		zap.S().Errorf("Error preparing copy to process value table statement: %s", err.Error())
		return messages, true, "Error preparing copy to process value table statement", err
	}

	zap.S().Debugf("pre exec")
	_, err = stmtCopyToPVT.Exec()
	zap.S().Debugf("post exec")
	if err != nil {
		errR := txn.Rollback()
		if errR != nil {
			zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
		}
		zap.S().Errorf("Error copying to process value table: %s", err.Error())
		return messages, true, "Error copying to process value table", err
	}

	zap.S().Debugf("pre close")
	err = stmtCopyToPVT.Close()
	if err != nil {
		errR := txn.Rollback()
		if errR != nil {
			zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
		}
		zap.S().Errorf("Error closing stmtCopytoPVT: %s", err.Error())
		return messages, true, "Error closing stmtCopytoPVT", err
	}
	zap.S().Debugf("post close")

	if isDryRun {
		err = txn.Rollback()
		if err != nil {
			return messages, true, "Failed to rollback", err
		}
		return putBackMsg, true, "AssetID not found", nil
	} else {
		zap.S().Debugf("Pre-commit to process value table: %d", toCommit)
		err = txn.Commit()
		if err != nil {
			return messages, true, "Failed to commit", err
		}
		zap.S().Debugf(
			"Committed %d messages, putting back %d messages",
			len(messages)-len(putBackMsg),
			len(putBackMsg))
		if len(putBackMsg) > 0 {
			return putBackMsg, true, "AssetID not found", nil
		}
		internal.KafkaPutBacks += float64(len(putBackMsg))
		internal.KafkaCommits += toCommit
	}
	return putBackMsg, false, "", nil
}
