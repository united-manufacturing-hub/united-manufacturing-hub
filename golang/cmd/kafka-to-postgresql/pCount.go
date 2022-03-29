package main

import (
	"context"
	"database/sql"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"time"
)

type Count struct{}

type count struct {
	Count       uint32 `json:"count"`
	Scrap       uint32 `json:"scrap"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

var CountMessagesToCommitLater = 0

// This processor will collect some count CountMessagesToCommitLater and then execute them, preventing database overload
func countProcessor() {
	zap.S().Debugf("Starting count processor")
	// Map structure: map[customerId][location][assetId][]message
	var messageMap map[string]map[string]map[string][]*kafka.Message
	messageMap = make(map[string]map[string]map[string][]*kafka.Message)

	tickerTime := 100 * time.Millisecond
	ticker := time.NewTicker(tickerTime)

	for !ShuttingDown || len(countChannel) > 0 {
		select {
		case <-ticker.C:
			if len(messageMap) == 0 {
				ticker.Reset(tickerTime)
				continue
			}

			var putBack map[string]map[string]map[string][]*kafka.Message
			zap.S().Debugf("Processing CountMessagesToCommitLater (ticker)")
			putBack = Count{}.ProcessMessages(messageMap)

			for _, m := range putBack {
				for _, m2 := range m {
					for _, messages := range m2 {
						for _, message := range messages {
							putBackChannel <- PutBackChan{msg: message, reason: "Count putback", errorString: nil}
						}
					}
				}
			}
			messageMap = make(map[string]map[string]map[string][]*kafka.Message)
			ticker.Reset(tickerTime)
			CountMessagesToCommitLater = 0
		case msg := <-countChannel:
			{
				parsed, parsedMsg := ParseMessage(msg, 0)
				if !parsed {
					continue
				}

				if messageMap[parsedMsg.CustomerId] == nil {
					messageMap[parsedMsg.CustomerId] = make(map[string]map[string][]*kafka.Message)
				}
				if messageMap[parsedMsg.CustomerId][parsedMsg.Location] == nil {
					messageMap[parsedMsg.CustomerId][parsedMsg.Location] = make(map[string][]*kafka.Message)
				}
				if messageMap[parsedMsg.CustomerId][parsedMsg.Location][parsedMsg.AssetId] == nil {
					messageMap[parsedMsg.CustomerId][parsedMsg.Location][parsedMsg.AssetId] = make([]*kafka.Message, 10000)
				}
				messageMap[parsedMsg.CustomerId][parsedMsg.Location][parsedMsg.AssetId] = append(messageMap[parsedMsg.CustomerId][parsedMsg.Location][parsedMsg.AssetId], msg)
				CountMessagesToCommitLater += 1
			}
		}
	}

	Count{}.ProcessMessages(messageMap)
}

func (c Count) ProcessMessages(messageMap map[string]map[string]map[string][]*kafka.Message) (putPack map[string]map[string]map[string][]*kafka.Message) {
	zap.S().Debug("Processing count CountMessagesToCommitLater")

	var txn *sql.Tx = nil
	txn, err := db.Begin()
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		putPack = messageMap
		return nil
	}

	ctxStmt, cancelStmt := context.WithDeadline(context.Background(), time.Now().Add(time.Second*1))
	defer cancelStmt()
	stmt := txn.StmtContext(ctxStmt, statement.InsertIntoCountTable)

	cntCommit := 0

	for customerID, customerMessageMap := range messageMap {
		for location, locationMessageMap := range customerMessageMap {
			for assetID, payload := range locationMessageMap {
				AssetTableID, success := GetAssetTableID(customerID, location, assetID)
				if !success {
					zap.S().Debugf("Asset table not found for customer %s, location %s, asset %s", customerID, location, assetID)
					putPack[customerID] = make(map[string]map[string][]*kafka.Message)
					putPack[customerID][location] = make(map[string][]*kafka.Message)
					putPack[customerID][location][assetID] = payload
					continue
				}

				for _, countMessage := range payload {
					var parsed, parsedMsg = ParseMessage(countMessage, 0)
					if !parsed {
						// If a message lands here, something is wrong !
						continue
					}

					var cnt count
					err = jsoniter.Unmarshal(parsedMsg.Payload, &cnt)
					if err != nil {
						// Ignore invalid CountMessagesToCommitLater
						continue
					}

					if cnt.Count == 0 {
						continue
					}
					_, err = stmt.Exec(AssetTableID, cnt.Count, cnt.Scrap, cnt.TimestampMs)
					if err != nil {
						zap.S().Errorf("Error inserting into count table: %s", err.Error())
						zap.S().Debugf("Payload: %#v", cnt)
						// Rollback failed inserts
						zap.S().Errorf("Rolling back failed inserts")
						_ = txn.Rollback()
						putPack = messageMap
						return
					}
				}
			}
		}
	}

	if isDryRun {
		zap.S().Debugf("Dry run: not committing transaction")
		err = txn.Rollback()
		if err != nil {
			putPack = messageMap
			return
		}
	} else {
		zap.S().Debugf("Committing %d transaction", cntCommit)
		err = txn.Commit()
		if err != nil {
			putPack = messageMap
			return
		}
		cntCommit += 1
	}

	Commits += cntCommit

	return putPack
}
