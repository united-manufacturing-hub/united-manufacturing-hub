package main

import (
	"context"
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"time"
)

type AddProduct struct{}

type addProduct struct {
	ProductId            string  `json:"product_id"`
	TimePerUnitInSeconds float64 `json:"time_per_unit_in_seconds"`
}

// ProcessMessages processes a AddProduct kafka message, by creating an database connection, decoding the json payload, retrieving the required additional database id's (like AssetTableID or ProductTableID) and then inserting it into the database and commiting
func (c AddProduct) ProcessMessages(msg ParsedMessage) (err error, putback bool) {

	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	// txnCtxCl is the cancel function of the context, used in the transaction creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnCtxCl()
	var txn *sql.Tx = nil
	txn, err = db.BeginTx(txnCtx, nil)
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return err, true
	}

	// sC is the payload, parsed as addProduct
	var sC addProduct
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

	ProductTableId, success := GetProductTableId(sC.ProductId, AssetTableID)

	// Changes should only be necessary between this marker

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	// txnStmtCtxCl is the cancel function of the context, used in the statement creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnStmtCtxCl()
	stmt := txn.StmtContext(txnStmtCtx, statement.InsertIntoProductTable)
	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	// stmtCtxCl is the cancel function of the context, used in the transactions execution creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer stmtCtxCl()
	_, err = stmt.ExecContext(stmtCtx, AssetTableID, ProductTableId, sC.TimePerUnitInSeconds)
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
