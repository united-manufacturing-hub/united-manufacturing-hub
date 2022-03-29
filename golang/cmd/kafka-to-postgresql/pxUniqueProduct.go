package main

import (
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

type UniqueProduct struct{}

type uniqueProduct struct {
	BeginTimestampMs           uint64 `json:"begin_timestamp_ms"`
	EndTimestampMs             int64  `json:"end_timestamp_ms"`
	ProductId                  string `json:"product_id"`
	IsScrap                    bool   `json:"is_scrap"`
	UniqueProductAlternativeID string `json:"uniqueProductAlternativeID"`
}

func (c UniqueProduct) ProcessMessages(msg ParsedMessage, pid int) (err error, putback bool) {

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		zap.S().Errorf("[%d] Error starting transaction: %s", pid, err.Error())
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

	stmt := txn.Stmt(statement.InsertIntoUniqueProductTable)
	_, err = stmt.Exec(AssetTableID, sC.BeginTimestampMs, NewNullInt64(sC.EndTimestampMs), ProductTableId, sC.IsScrap, sC.UniqueProductAlternativeID)
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
