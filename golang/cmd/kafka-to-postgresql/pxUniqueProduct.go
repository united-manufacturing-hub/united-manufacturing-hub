package main

import (
	"context"
	"database/sql"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	kafka2 "github.com/united-manufacturing-hub/umh-lib/v2/kafka"
	"github.com/united-manufacturing-hub/umh-lib/v2/other"
	"go.uber.org/zap"
	"time"
)

type UniqueProduct struct{}

type uniqueProduct struct {
	BeginTimestampMs           *uint64 `json:"begin_timestamp_ms"`
	EndTimestampMs             *int64  `json:"end_timestamp_ms"`
	ProductId                  *string `json:"product_id"`
	IsScrap                    *bool   `json:"isScrap"`
	UniqueProductAlternativeID *string `json:"uniqueProductAlternativeID"`
}

// ProcessMessages processes a UniqueProduct kafka message, by creating an database connection, decoding the json payload, retrieving the required additional database id's (like AssetTableID or ProductTableID) and then inserting it into the database and commiting
func (c UniqueProduct) ProcessMessages(msg kafka2.ParsedMessage) (putback bool, err error) {

	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(other.FiveSeconds))
	// txnCtxCl is the cancel function of the context, used in the transaction creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnCtxCl()
	var txn *sql.Tx = nil
	txn, err = db.BeginTx(txnCtx, nil)
	if err != nil {
		zap.S().Errorf("Error starting transaction: %s", err.Error())
		return true, err
	}

	// sC is the payload, parsed as uniqueProduct
	var sC uniqueProduct
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		// Ignore malformed messages
		zap.S().Warnf("Failed to unmarshal message: %s", err.Error())
		return false, err
	}
	if !other.IsValidStruct(sC, []string{"EndTimestampMs"}) {
		zap.S().Warnf("Invalid message: %s, inserting into putback !", string(msg.Payload))
		return true, nil
	}
	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		return true, fmt.Errof("Failed to get AssetTableID for CustomerId: %s, Location: %s, AssetId: %s", msg.CustomerId, msg.Location, msg.AssetId)
	}

	var ProductTableId uint32
	ProductTableId, success = GetProductTableId(*sC.ProductId, AssetTableID)
	if !success {
		return true, fmt.Errof("Failed to get ProductTableId for ProductId: %s, AssetTableID: %d", *sC.ProductId, AssetTableID)
	}

	// Changes should only be necessary between this marker

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(other.FiveSeconds))
	// txnStmtCtxCl is the cancel function of the context, used in the statement creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnStmtCtxCl()
	stmt := txn.StmtContext(txnStmtCtx, statement.InsertIntoUniqueProductTable)
	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(other.FiveSeconds))
	// stmtCtxCl is the cancel function of the context, used in the transactions execution creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer stmtCtxCl()
	_, err = stmt.ExecContext(stmtCtx, AssetTableID, sC.BeginTimestampMs, NewNullInt64(*sC.EndTimestampMs), ProductTableId, sC.IsScrap, sC.UniqueProductAlternativeID)
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
