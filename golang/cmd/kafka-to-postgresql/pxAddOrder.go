package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

type AddOrder struct{}

type addOrder struct {
	ProductId   *string `json:"product_id"`
	OrderId     *string `json:"order_id"`
	TargetUnits *uint64 `json:"target_units"`
}

// ProcessMessages processes a AddOrder kafka message, by creating an database connection, decoding the json payload, retrieving the required additional database id's (like AssetTableID or ProductTableID) and then inserting it into the database and commiting
func (c AddOrder) ProcessMessages(msg internal.ParsedMessage) (putback bool, err error) {

	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnCtxCl is the cancel function of the context, used in the transaction creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnCtxCl()
	var txn *sql.Tx = nil
	txn, err = db.BeginTx(txnCtx, nil)
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return true, err
	}

	// sC is the payload, parsed as addOrder
	var sC addOrder
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		// Ignore malformed messages
		zap.S().Warnf("Failed to unmarshal message: %s", err.Error())
		return false, err
	}
	if !internal.IsValidStruct(sC, []string{}) {
		zap.S().Warnf("Invalid message: %s, discarding !", string(msg.Payload))
		return true, nil
	}
	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		return true, errors.New(fmt.Sprintf("Failed to get AssetTableID for CustomerId: %s, Location: %s, AssetId: %s", msg.CustomerId, msg.Location, msg.AssetId))
	}

	ProductTableID, success := GetProductTableId(*sC.ProductId, AssetTableID)
	if !success {
		return true, errors.New(fmt.Sprintf("Failed to get ProductTableID for ProductId: %s, AssetTableID: %d", *sC.ProductId, AssetTableID))
	}

	// Changes should only be necessary between this marker

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnStmtCtxCl is the cancel function of the context, used in the statement creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnStmtCtxCl()
	stmt := txn.StmtContext(txnStmtCtx, statement.UpdateCountTableScrap)
	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// stmtCtxCl is the cancel function of the context, used in the transactions execution creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer stmtCtxCl()
	_, err = stmt.ExecContext(stmtCtx, sC.OrderId, ProductTableID, sC.TargetUnits, AssetTableID)
	if err != nil {
		return true, err
	}

	// And this marker

	if isDryRun {
		zap.S().Debugf("Dry run: not committing transaction")
		err = txn.Rollback()
		if err != nil {
			return true, err
		}
	} else {

		err = txn.Commit()
		if err != nil {
			return true, err
		}
	}

	return false, err
}
