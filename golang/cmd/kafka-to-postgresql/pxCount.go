package main

import (
	"context"
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

type Count struct{}

type count struct {
	Count       uint32 `json:"count"`
	Scrap       uint32 `json:"scrap:omitempty"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// ProcessMessages processes a Count kafka message, by creating an database connection, decoding the json payload, retrieving the required additional database id's (like AssetTableID or ProductTableID) and then inserting it into the database and commiting
func (c Count) ProcessMessages(msg ParsedMessage) (err error, putback bool) {

	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnCtxCl is the cancel function of the context, used in the transaction creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnCtxCl()
	var txn *sql.Tx = nil
	txn, err = db.BeginTx(txnCtx, nil)
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return err, true
	}

	// sC is the payload, parsed as count
	var sC count
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		// Ignore malformed messages
		zap.S().Warnf("Failed to unmarshal message: %s", err.Error())
		return err, false
	}

	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		return nil, true
	}

	// Changes should only be necessary between this marker

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnStmtCtxCl is the cancel function of the context, used in the statement creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnStmtCtxCl()

	stmt := txn.StmtContext(txnStmtCtx, statement.InsertIntoCountTable)

	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// stmtCtxCl is the cancel function of the context, used in the transactions execution creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer stmtCtxCl()

	_, err = stmt.ExecContext(stmtCtx, AssetTableID, sC.Count, sC.Scrap, sC.TimestampMs)
	if err != nil {
		//zap.S().Debugf("Error inserting into count table: %s", err.Error())
		return err, true
	}

	// And this marker

	if isDryRun {
		//zap.S().Debugf("Dry run: not committing transaction")
		err = txn.Rollback()
		if err != nil {
			zap.S().Errorf("Error rolling back transaction: %s", err.Error())
			return err, true
		}
	} else {
		//zap.S().Debugf("Committing transaction")
		err = txn.Commit()
		if err != nil {
			zap.S().Errorf("Error committing transaction: %s", err.Error())
			return err, true
		}
	}

	//zap.S().Debugf("Successfully processed count message: %v", msg)
	return err, false
}
