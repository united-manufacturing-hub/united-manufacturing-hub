// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

// Contains timestamp_ms and 1 other key, which is a string
type processValueString map[string]interface{}

var processValueStringChannel chan *kafka.Message

// startProcessValueChannel reads messages from the processValueStringChannel and inserts them into a temporary buffer, before committing them to the database
func startProcessValueStringQueueAggregator() {
	chanSize := 500_000
	if os.Getenv("PVS_CHANNEL_SIZE") != "" {
		atoi, err := strconv.Atoi(os.Getenv("PVS_CHANNEL_SIZE"))
		if err != nil {
			zap.S().Warnf("[HT][PVS] PVS_CHANNEL_SIZE is not a valid integer: %s", err.Error())
		}
		chanSize = atoi
	}
	writeToDbTimer := time.NewTicker(time.Second * 5)
	if os.Getenv("PVS_WRITE_TO_DB_INTERVAL") != "" {
		atoi, err := strconv.Atoi(os.Getenv("PVS_WRITE_TO_DB_INTERVAL"))
		if err != nil {
			zap.S().Warnf("[HT][PVS] PV_WRITE_TO_DB_INTERVAL is not a valid integer: %s", err.Error())
		}
		writeToDbTimer = time.NewTicker(time.Second * time.Duration(atoi))
	}

	// This channel is used to aggregate messages from the kafka queue, for further processing
	// It size was chosen, to prevent timescaledb from choking on large inserts
	processValueStringChannel = make(chan *kafka.Message, chanSize)

	messages := make([]*kafka.Message, 0)

	// Goal: 5k messages per commit and commit every 5 seconds even if there are less than 5k messages

	for !ShuttingDown {
		select {
		case msg := <-processValueStringChannel: // Receive message from channel
			{
				messages = append(messages, msg)
				// This checks for >= 5000, because we don't want to block the channel (see size of the processValueChannel)
				if len(messages) >= chanSize {
					// zap.S().Debugf("[HT][PVS] KafkaMessages length: %d", len(messages))
					putBackMsg, putback, reason, err := writeProcessValueStringToDatabase(messages)
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
		case <-writeToDbTimer.C: // Commit data into db
			{
				// zap.S().Debugf("[HT][PVS] KafkaMessages length: %d", len(messages))
				if len(messages) == 0 {

					continue
				}
				putBackMsg, putback, reason, err := writeProcessValueStringToDatabase(messages)
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

func writeProcessValueStringToDatabase(messages []*kafka.Message) (
	putBackMsg []*kafka.Message,
	putback bool,
	reason string,
	err error) {
	zap.S().Debugf("[HT][PVS] Writing %d messages to database", len(messages))
	var txn *sql.Tx
	txn, err = db.Begin()
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return messages, true, "Error starting transaction", err
	}

	zap.S().Debugf("[HT][PVS] Creating temporary table")
	{
		stmt := txn.Stmt(statement.CreateTmpProcessValueTableString)
		_, err = stmt.Exec()
		if err != nil {
			errR := txn.Rollback()
			if errR != nil {
				zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
			}
			zap.S().Errorf("Error creating temporary table: %s", err.Error())
			return messages, true, "Error creating temporary table", err
		}
	}

	putBackMsg = make([]*kafka.Message, 0)
	// toCommit is used for stats only, it just increments, whenever a message was added to the transaction.
	// at the end, this count is added to the global Commit counter
	toCommit := float64(0)
	{

		zap.S().Debugf("[HT][PVS] Preparing copy statement")
		var stmtCopy *sql.Stmt
		stmtCopy, err = txn.Prepare(
			pq.CopyIn(
				"tmp_processvaluestringtable",
				"timestamp",
				"asset_id",
				"value",
				"valuename"))
		if err != nil {
			errR := txn.Rollback()
			if errR != nil {
				zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
			}
			zap.S().Errorf("Error preparing copy statement: %s", err.Error())
			return messages, true, "Error preparing copy statement", err
		}

		zap.S().Debugf("[HT][PVS] Copying %d messages to temporary table", len(messages))
		// Copy into the temporary table
		for _, message := range messages {
			couldParse, parsedMessage := internal.ParseMessage(message)
			if !couldParse {
				zap.S().Errorf("[HT][PVS] Could not parse message: %s", message.String())
				putBackMsg = append(putBackMsg, message)
				continue
			}

			// sC is the payload, parsed as processValueString
			var sC processValueString
			err = jsoniter.Unmarshal(parsedMessage.Payload, &sC)
			if err != nil {
				zap.S().Errorf("[HT][PVS] Could not unmarshal message: %s", err.Error())
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

			if timestampString, timestampInParsedMessagePayload := sC["timestamp_ms"]; timestampInParsedMessagePayload {
				var tsF64 float64
				tsF64, err = getFloat(timestampString)

				if err != nil {
					zap.S().Debugf("[HT][PVS] Could not parse timestamp: %s", err.Error())
					continue
				}
				timestampMs := uint64(tsF64)
				for k, v := range sC {
					switch k {
					case "timestamp_ms":
					// Copied these exceptions from mqtt-to-postgresql
					// These are here for historical reasons
					case "measurement":
					case "serial_number":
					default:
						value, valueIsString := v.(string)
						if !valueIsString {

							// zap.S().Debugf("Value is not string")
							// Value is malformed, skip to next key
							continue
						}

						// This coversion is necessary for postgres
						timestamp := time.Unix(0, int64(timestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")
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
		}

		zap.S().Debugf("[HT][PVS] Copied %d messages to temporary table", toCommit)

		err = stmtCopy.Close()
		zap.S().Debugf("[HT][PVS] Closed copy statement")
		if err != nil {
			errR := txn.Rollback()
			if errR != nil {
				zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
			}
			return messages, true, "Failed to close copy statement", err
		}
	}

	zap.S().Debugf("[HT][PVS] Preparing insert statement")
	var stmtCopyToPVTS *sql.Stmt
	stmtCopyToPVTS, err = txn.Prepare(
		`
			INSERT INTO processvaluestringtable (SELECT * FROM tmp_processvaluestringtable) ON CONFLICT DO NOTHING;
		`)
	if err != nil {
		errR := txn.Rollback()
		if errR != nil {
			zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
		}
		zap.S().Errorf("Error preparing copy to process value table statement: %s", err.Error())
		return messages, true, "Error preparing copy to process value table statement", err
	}

	zap.S().Debugf("[HT][PVS] Executing insert statement")
	_, err = stmtCopyToPVTS.Exec()
	if err != nil {
		errR := txn.Rollback()
		if errR != nil {
			zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
		}
		zap.S().Errorf("Error copying to process value table: %s", err.Error())
		return messages, true, "Error copying to process value table", err
	}

	err = stmtCopyToPVTS.Close()
	if err != nil {
		errR := txn.Rollback()
		if errR != nil {
			zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
		}
		zap.S().Errorf("Error closing stmtCopytoPVTS: %s", err.Error())
		return messages, true, "Error closing stmtCopytoPVTS", err
	}

	if isDryRun {
		err = txn.Rollback()
		if err != nil {
			return messages, true, "Failed to rollback", err
		}
		if len(putBackMsg) > 0 {
			return putBackMsg, true, AssetIDnotFound, nil
		}
	} else {
		zap.S().Debugf("[HT][PVS] Committing transaction")
		err = txn.Commit()
		zap.S().Debugf("[HT][PVS] Committed transaction")
		if err != nil {
			return messages, true, "Failed to commit", err
		}
		// zap.S().Debugf("Committed %d messages, putting back %d messages", len(messages)-len(putBackMsg), len(putBackMsg))
		if len(putBackMsg) > 0 {
			return putBackMsg, true, AssetIDnotFound, nil
		}
		internal.KafkaPutBacks += float64(len(putBackMsg))
		internal.KafkaCommits += toCommit
	}

	return putBackMsg, false, "", nil
}
