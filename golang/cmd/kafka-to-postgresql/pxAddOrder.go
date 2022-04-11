package main

import (
	"context"
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"time"
)

type AddOrder struct{}

type addOrder struct {
	ProductId   string `json:"product_id"`
	OrderId     string `json:"order_id"`
	TargetUnits uint64 `json:"target_units"`
}

func (c AddOrder) ProcessMessages(msg ParsedMessage) (err error, putback bool) {

	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnCtxCl()
	var txn *sql.Tx = nil
	txn, err = db.BeginTx(txnCtx, nil)
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return err, true
	}

	var sC addOrder
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		// Ignore malformed messages
		return err, false
	}
	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		return nil, true
	}

	ProductTableID, success := GetProductTableId(sC.ProductId, AssetTableID)
	if !success {
		return nil, true
	}

	// Changes should only be necessary between this marker

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnStmtCtxCl()
	stmt := txn.StmtContext(txnStmtCtx, statement.UpdateCountTableScrap)
	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer stmtCtxCl()
	_, err = stmt.ExecContext(stmtCtx, sC.OrderId, ProductTableID, sC.TargetUnits, AssetTableID)
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
