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
			if CountMessagesToCommitLater == 0 {
				ticker.Reset(tickerTime)
				continue
			}

			var putBack map[string]map[string]map[string][]*kafka.Message
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

func (c Count) ProcessMessages(messageMap map[string]map[string]map[string][]*kafka.Message) (putBackMm map[string]map[string]map[string][]*kafka.Message) {

	var txn *sql.Tx = nil
	txn, err := db.Begin()
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		putBackMm = messageMap
		return
	}

	// Create tmp count table
	ctxStmt, cancelStmt := context.WithDeadline(context.Background(), time.Now().Add(time.Second*1))
	defer cancelStmt()
	stmt := txn.StmtContext(ctxStmt, statement.CreateTmpCountTable)
	_, err = stmt.Exec()
	if err != nil {
		zap.S().Errorf("Error creating tmp count table: %s", err.Error())
		putBackMm = messageMap
		return
	}

	cntCommit := 0
	invalidMessages := 0

	ctxPrepCtx, cancelPrepCtx := context.WithDeadline(context.Background(), time.Now().Add(time.Second*1))
	defer cancelPrepCtx()
	var stmtCopyIn *sql.Stmt
	stmtCopyIn, err = txn.PrepareContext(ctxPrepCtx, pq.CopyIn("tmp_counttable", "timestamp", "asset_id", "count", "scrap"))

	// Insert everything into copy table
	for customerID, customerMessageMap := range messageMap {
		for location, locationMessageMap := range customerMessageMap {
			for assetID, payload := range locationMessageMap {
				AssetTableID, success := GetAssetTableID(customerID, location, assetID)
				if !success {
					zap.S().Debugf("Asset table not found for customer %s, location %s, asset %s", customerID, location, assetID)
					putBackMm[customerID] = make(map[string]map[string][]*kafka.Message)
					putBackMm[customerID][location] = make(map[string][]*kafka.Message)
					putBackMm[customerID][location][assetID] = payload
					continue
				}

				for _, countMessage := range payload {
					var parsed, parsedMsg = ParseMessage(countMessage, 0)
					if !parsed {
						invalidMessages += 1
						// If a message lands here, something is wrong !
						continue
					}

					var cnt count
					err = jsoniter.Unmarshal(parsedMsg.Payload, &cnt)
					if err != nil {
						invalidMessages += 1
						// Ignore invalid CountMessagesToCommitLater
						continue
					}

					if cnt.Count == 0 {
						invalidMessages += 1
						continue
					}
					timestamp := time.Unix(0, int64(cnt.TimestampMs*uint64(1000000))).Format("2006-01-02T15:04:05.000Z")

					_, err = stmtCopyIn.Exec(timestamp, AssetTableID, cnt.Count, cnt.Scrap)
					if err != nil {
						zap.S().Errorf("Error inserting into count table: %s", err.Error())
						zap.S().Debugf("Payload: %#v", cnt)
						// Rollback failed inserts
						zap.S().Errorf("Rolling back failed inserts")
						_ = txn.Rollback()
						putBackMm = messageMap
						return
					}
					cntCommit += 1
				}
			}
		}
	}
	err = stmtCopyIn.Close()
	if err != nil {
		putBackMm = messageMap
		return
	}

	var stmtInsertFromCopyTable *sql.Stmt
	stmtInsertFromCopyTable, err = txn.Prepare(`
			INSERT INTO counttable (SELECT * FROM tmp_counttable) ON CONFLICT DO NOTHING;
		`)

	if err != nil {
		putBackMm = messageMap
		return
	}

	_, err = stmtInsertFromCopyTable.Exec()
	if err != nil {
		putBackMm = messageMap
		return
	}

	err = stmtInsertFromCopyTable.Close()
	if err != nil {
		putBackMm = messageMap
		return
	}

	if isDryRun {
		zap.S().Debugf("Dry run: not committing transaction")
		err = txn.Rollback()
		if err != nil {
			putBackMm = messageMap
			return
		}
	} else {
		zap.S().Debugf("Committing %d transaction", cntCommit)
		err = txn.Commit()
		if err != nil {
			putBackMm = messageMap
			return
		}
	}

	Commits += cntCommit

	return putBackMm
}
