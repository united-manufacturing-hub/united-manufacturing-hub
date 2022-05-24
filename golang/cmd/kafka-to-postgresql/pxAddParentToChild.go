package main

import (
	"context"
	"database/sql"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

type AddParentToChild struct{}

type addParentToChild struct {
	ChildAID    *string `json:"childAID"`
	ParentAID   *string `json:"parentAID"`
	TimestampMs *uint64 `json:"timestamp_ms"`
}

// ProcessMessages processes a AddParentToChild kafka message, by creating an database connection, decoding the json payload, retrieving the required additional database id's (like AssetTableID or ProductTableID) and then inserting it into the database and commiting
func (c AddParentToChild) ProcessMessages(msg internal.ParsedMessage) (putback bool, err error, forcePbTopic bool) {

	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnCtxCl is the cancel function of the context, used in the transaction creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnCtxCl()
	var txn *sql.Tx = nil
	txn, err = db.BeginTx(txnCtx, nil)
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return true, err, false
	}

	isCommited := false
	defer func() {
		if !isCommited && !isDryRun {
			err = txn.Rollback()
			if err != nil {
				zap.S().Errorf("Error rolling back transaction: %s", err.Error())
			}
		}
	}()

	// sC is the payload, parsed as addParentToChild
	var sC addParentToChild
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		zap.S().Warnf("Failed to unmarshal message: %s", err.Error())
		return false, err, true
	}
	if !internal.IsValidStruct(sC, []string{}) {
		zap.S().Warnf("Invalid message: %s, inserting into putback !", string(msg.Payload))
		return true, nil, true
	}
	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		zap.S().Warnf("Failed to get AssetTableID")
		return true, fmt.Errorf("failed to get AssetTableID for CustomerId: %s, Location: %s, AssetId: %s", msg.CustomerId, msg.Location, msg.AssetId), false
	}

	// Changes should only be necessary between this marker
	var ChildUID uint32
	ChildUID, success = GetUniqueProductID(*sC.ChildAID, AssetTableID)
	if !success {
		return true, fmt.Errorf("failed to get UniqueProductID for ChildAID: %s, AssetTableID: %d", *sC.ChildAID, AssetTableID), false
	}

	var ParentUID uint32
	ParentUID, success = GetLatestParentUniqueProductID(*sC.ParentAID, AssetTableID)
	if !success {
		return true, fmt.Errorf("failed to get LatestParentUniqueProductID for ParentAID: %s, AssetTableID: %d", *sC.ParentAID, AssetTableID), false
	}

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnStmtCtxCl is the cancel function of the context, used in the statement creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnStmtCtxCl()
	stmt := txn.StmtContext(txnStmtCtx, statement.InsertIntoProductInheritanceTable)
	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// stmtCtxCl is the cancel function of the context, used in the transactions execution creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer stmtCtxCl()
	_, err = stmt.ExecContext(stmtCtx, ParentUID, ChildUID, sC.TimestampMs)
	if err != nil {

		zap.S().Errorf("Error executing statement: %s", err.Error())
		return true, err, false
	}

	// And this marker

	if isDryRun {
		zap.S().Debugf("Dry run: not committing transaction")
		err = txn.Rollback()
		if err != nil {
			return true, err, false
		}
	} else {
		zap.S().Debugf("Committing transaction")
		err = txn.Commit()
		if err != nil {
			zap.S().Errorf("Error committing transaction: %s", err.Error())
			return true, err, false
		}
		isCommited = true
	}

	return false, err, false
}
