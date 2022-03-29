package main

import (
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

type ProductTag struct{}

type productTag struct {
	AID  string `json:"AID"`
	Name string `json:"name"`
	// TODO: Value is not correctly defined in the docs, i assume float64 just to be safe
	Value       float64 `json:"value"`
	TimestampMs uint64  `json:"timestamp_ms"`
}

func (c ProductTag) ProcessMessages(msg ParsedMessage, pid int) (err error, putback bool) {

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		zap.S().Errorf("[%d] Error starting transaction: %s", pid, err.Error())
		return err, true
	}

	var sC productTag
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
	ProductTableId, success = GetUniqueProductID(sC.AID, AssetTableID)
	if !success {
		return nil, true
	}

	// Changes should only be necessary between this marker

	stmt := txn.Stmt(statement.InsertIntoProductTagTable)
	_, err = stmt.Exec(sC.Name, sC.Value, sC.TimestampMs, ProductTableId)
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
