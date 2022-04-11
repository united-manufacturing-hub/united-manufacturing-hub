package main

import (
	"context"
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
	"time"
)

type UniqueProduct struct{}

type uniqueProduct struct {
	BeginTimestampMs           uint64 `json:"begin_timestamp_ms"`
	EndTimestampMs             int64  `json:"end_timestamp_ms"`
	ProductId                  string `json:"product_id"`
	IsScrap                    bool   `json:"is_scrap"`
	UniqueProductAlternativeID string `json:"uniqueProductAlternativeID"`
}

func (c UniqueProduct) ProcessMessages(msg ParsedMessage) (err error, putback bool) {

	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnCtxCl()
	var txn *sql.Tx = nil
	txn, err = db.BeginTx(txnCtx, nil)
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return err, true
	}

	var sC uniqueProduct
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		// Ignore malformed messages
		return err, false
	}
	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		return nil, true
	}

	var ProductTableId uint32
	ProductTableId, success = GetProductTableId(sC.ProductId, AssetTableID)
	if !success {
		return nil, true
	}

	// Changes should only be necessary between this marker

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer txnStmtCtxCl()
	stmt := txn.StmtContext(txnStmtCtx, statement.InsertIntoUniqueProductTable)
	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer stmtCtxCl()
	_, err = stmt.ExecContext(stmtCtx, AssetTableID, sC.BeginTimestampMs, NewNullInt64(sC.EndTimestampMs), ProductTableId, sC.IsScrap, sC.UniqueProductAlternativeID)
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
