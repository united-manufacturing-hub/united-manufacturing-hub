package main

import (
	"context"
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"time"
)

type AddParentToChild struct{}

type addParentToChild struct {
	ChildAID    string `json:"childAID"`
	ParentAID   string `json:"parentAID"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

func (c AddParentToChild) ProcessMessages(msg ParsedMessage) (err error, putback bool) {

	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnCtxCl()
	var txn *sql.Tx = nil
	txn, err = db.BeginTx(txnCtx, nil)
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return err, true
	}

	var sC addParentToChild
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		// Ignore malformed messages
		return err, false
	}
	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		return nil, true
	}

	// Changes should only be necessary between this marker
	var ChildUID uint32
	ChildUID, success = GetUniqueProductID(sC.ChildAID, AssetTableID)
	if !success {
		return nil, true
	}

	var ParentUID uint32
	ParentUID, success = GetLatestParentUniqueProductID(sC.ParentAID, AssetTableID)
	if !success {
		return nil, true
	}

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnStmtCtxCl()
	stmt := txn.StmtContext(txnStmtCtx, statement.InsertIntoProductInheritanceTable)
	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer stmtCtxCl()
	_, err = stmt.ExecContext(stmtCtx, ParentUID, ChildUID, sC.TimestampMs)
	if err != nil {
		return err, true
	}

	// And this marker

	if isDryRun {
		zap.S().Debugf("Dry run: not committing transaction")
		err = txn.Rollback()
		if err != nil {
			return err, true
		}
	} else {

		err = txn.Commit()
		if err != nil {
			return err, true
		}
	}

	return err, false
}
