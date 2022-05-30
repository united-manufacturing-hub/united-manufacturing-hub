package main

/*
	Warning:
		This file is based on old source code, not on documentation !
*/
import (
	"context"
	"database/sql"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

type ModifyProducesPieces struct{}

type modifyProducesPieces struct {
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Count *int32 `json:"count"`
	// Has to be int32 to allow transmission of "not changed" value (value < 0)
	Scrap *int32 `json:"scrap"`
}

// ProcessMessages processes a ModifyProducesPieces kafka message, by creating an database connection, decoding the json payload, retrieving the required additional database id's (like AssetTableID or ProductTableID) and then inserting it into the database and commiting
func (c ModifyProducesPieces) ProcessMessages(msg internal.ParsedMessage) (putback bool, err error, forcePbTopic bool) {

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
			} else {
				zap.S().Warnf("Rolled back transaction !")
			}
		}
	}()

	// sC is the payload, parsed as modifyProducesPieces
	var sC modifyProducesPieces
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		zap.S().Warnf("Failed to unmarshal message: %s", err.Error())
		return false, err, true
	}
	if !internal.IsValidStruct(sC, []string{"Count", "Scrap"}) {
		zap.S().Warnf("Invalid message: %s, inserting into putback !", string(msg.Payload))
		return true, nil, true
	}

	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		zap.S().Warnf("Failed to get AssetTableID")
		return true, fmt.Errorf("failed to get AssetTableID for CustomerId: %s, Location: %s, AssetId: %s", msg.CustomerId, msg.Location, msg.AssetId), false
	}

	// Changes should only be necessary between this marker

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnStmtCtxCl is the cancel function of the context, used in the statement creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnStmtCtxCl()

	stmtUpdateCAndS := txn.StmtContext(txnStmtCtx, statement.UpdateCountTableSetCountAndScrapByAssetId)
	stmtUpdateC := txn.StmtContext(txnStmtCtx, statement.UpdateCountTableSetCountByAssetId)
	stmtUpdateS := txn.StmtContext(txnStmtCtx, statement.UpdateCountTableSetScrapByAssetId)

	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// stmtCtxCl is the cancel function of the context, used in the transactions execution creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer stmtCtxCl()

	if sC.Count != nil && *sC.Count >= 0 {
		if sC.Scrap != nil && *sC.Scrap >= 0 {
			_, err = stmtUpdateCAndS.ExecContext(stmtCtx, *sC.Count, *sC.Scrap, AssetTableID)
			if err != nil {
				zap.S().Errorf("Failed to update count and scrap: %s", err.Error())
				return true, err, false
			}
		} else {
			_, err = stmtUpdateC.ExecContext(stmtCtx, *sC.Count, AssetTableID)
			if err != nil {
				zap.S().Errorf("Failed to update count: %s", err.Error())
				return true, err, false
			}
		}
	} else if sC.Scrap != nil && *sC.Scrap >= 0 {
		_, err = stmtUpdateS.ExecContext(stmtCtx, *sC.Scrap, AssetTableID)
		if err != nil {
			zap.S().Errorf("Failed to update scrap: %s", err.Error())
			return true, err, false
		}
	}
	// And this marker

	if isDryRun {
		zap.S().Debugf("Dry run: not committing transaction")
		err = txn.Rollback()
		if err != nil {
			zap.S().Errorf("Error rolling back transaction: %s", err.Error())
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

	zap.S().Debugf("Successfully processed modifyProducesPieces message: %v", msg)
	return false, err, false
}
