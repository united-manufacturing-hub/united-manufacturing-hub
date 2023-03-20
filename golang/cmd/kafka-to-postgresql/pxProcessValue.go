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
	"strings"
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
					// zap.S().Debugf("[HT][PV][AA] KafkaMessages length: %d", len(messages))
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
				// zap.S().Debugf("[HT][PV] KafkaMessages length: %d", len(messages))
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
	var txn *sql.Tx
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
	// temporaryCommitCounterAsFloat64 is used for stats only, it just increments, whenever a message was added to the transaction.
	// at the end, this count is added to the global Commit counter
	temporaryCommitCounterAsFloat64 := float64(0)
	discardCounter := 0
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
			// zap.S().Debugf("[HT][PV] Timestamp: %d", timestampMs)
			for k, v := range sC {
				// Ignore null values
				if v == nil {
					zap.S().Debugf("[HT][PV] Ignoring null value [%s/%s/%s/%s]", parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, k)
					continue
				}
				switch k {
				case "timestamp_ms":
				// Copied these exceptions from mqtt-to-postgresql
				// These are here for historical reasons
				case "measurement":
				case "serial_number":
				default:
					var value float64

					// This function can parse any float, int and bool values.
					// It will discard any other values with a warning.
					switch t := v.(type) {
					case float64:
						// This converts normal int and float values to float64
						value = t
					case string:
						valAsStr := t
						// https://go.dev/ref/spec#Floating-point_literals
						value, err = strconv.ParseFloat(valAsStr, 64)
						if err != nil {
							// convert string to lower case
							// Passes all tests cases from https://go.dev/ref/spec#Integer_literals
							valAsStr = strings.ToLower(valAsStr)
							if len(valAsStr) > 2 && (valAsStr[0:2] == "0x" || valAsStr[0:2] == "0o" || valAsStr[0:2] == "0b") {
								// parse hex
								var parseInt int64
								parseInt, err = strconv.ParseInt(valAsStr, 0, 64)
								if err != nil {
									zap.S().Warnf("Error parsing %s string: %s (%s)\n", valAsStr[0:2], valAsStr, err)
									discardCounter++
									continue
								}
								value = float64(parseInt)
							} else if valAsStr == "true" {
								value = 1
							} else if valAsStr == "false" {
								value = 0
								// German float style
							} else if strings.Contains(valAsStr, ",") {
								valAsStr = strings.ReplaceAll(valAsStr, ",", ".")
								value, err = strconv.ParseFloat(valAsStr, 64)
								if err != nil {
									zap.S().Warnf("error parsing %s as float64: %s [%s/%s/%s/%s]\n", valAsStr, err, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, k)
									discardCounter++
									continue
								} else {
									zap.S().Warnf("Encountered german float style: %s [%s/%s/%s/%s]\n", valAsStr, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, k)
								}
							} else {
								zap.S().Warnf("error parsing %s as float64: %s [%s/%s/%s/%s]\n", valAsStr, err, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, k)
								discardCounter++
								continue
							}
						}
					case bool:
						boolVal := t
						if boolVal {
							value = 1
						} else {
							value = 0
						}
					default:
						zap.S().Warnf("[HT][PV] Value is not a string, float64 or boolean (%v) [%s/%s/%s/%s]", v, parsedMessage.CustomerId, parsedMessage.Location, parsedMessage.AssetId, k)
					}

					// This coversion is necessary for postgres
					timestamp := time.Unix(0, int64(timestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")

					// zap.S().Debugf("[HT][PV] Inserting %d, %s, %f, %s", AssetTableID, timestamp, value, k)
					_, err = stmtCopy.Exec(timestamp, AssetTableID, value, k)
					if err != nil {
						zap.S().Errorf("Error inserting into temporary table: %s", err.Error())
						errR := txn.Rollback()
						if errR != nil {
							zap.S().Errorf("Failed to rollback tx !: %s", err.Error())
						}
						return messages, true, "Error inserting into temporary table", err
					}
					temporaryCommitCounterAsFloat64 += 1
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
		return putBackMsg, true, AssetIDnotFound, nil
	} else {
		zap.S().Debugf("Pre-commit to process value table: %f", temporaryCommitCounterAsFloat64)
		err = txn.Commit()
		if err != nil {
			return messages, true, "Failed to commit", err
		}
		zap.S().Debugf(
			"Committed %d messages, putting back %d messages",
			len(messages)-len(putBackMsg)-discardCounter,
			len(putBackMsg))
		if discardCounter > 0 {
			zap.S().Warnf("Discarded %d messages", discardCounter)
		}
		if len(putBackMsg) > 0 {
			return putBackMsg, true, AssetIDnotFound, nil
		}
		internal.KafkaPutBacks += float64(len(putBackMsg))
		internal.KafkaCommits += temporaryCommitCounterAsFloat64
	}
	return putBackMsg, false, "", nil
}

const AssetIDnotFound = "AssetID not found"
