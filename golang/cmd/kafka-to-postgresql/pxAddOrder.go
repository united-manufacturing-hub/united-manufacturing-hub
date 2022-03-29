package main

import (
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

type AddOrder struct{}

type addOrder struct {
	ProductId   string `json:"product_id"`
	OrderId     string `json:"order_id"`
	TargetUnits uint64 `json:"target_units"`
}

func (c AddOrder) ProcessMessages(msg ParsedMessage, pid int) (err error, putback bool) {

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		zap.S().Errorf("[%d] Error starting transaction: %s", pid, err.Error())
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

	stmt := txn.Stmt(statement.UpdateCountTableScrap)
	_, err = stmt.Exec(sC.OrderId, ProductTableID, sC.TargetUnits, AssetTableID)
	if err != nil {
		return err, true
	}

	// And this marker

	if isDryRun {
		zap.S().Debugf("[%d] Dry run: not committing transaction", pid)
		err = txn.Rollback()
		if err != nil {
			return err, true
		}
	} else {
		zap.S().Debugf("[%d] Committing transaction", pid)
		err = txn.Commit()
		if err != nil {
			return err, true
		}
	}

	return err, true
}
