package main

import (
	"database/sql"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

type AddShift struct{}

type addShift struct {
	TimestampMsEnd uint64 `json:"timestamp_ms_end"`
	TimestampMs    uint64 `json:"timestamp_ms"`
}

func (c AddShift) ProcessMessages(msg ParsedMessage, pid int) (err error, putback bool) {

	var txn *sql.Tx = nil
	txn, err = db.Begin()
	if err != nil {
		zap.S().Errorf("[%d] Error starting transaction: %s", pid, err.Error())
		return err, true
	}

	var sC addShift
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		// Ignore malformed messages
		return err, false
	}
	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		return nil, true
	}

	// Changes should only be necessary between this marker

	stmt := txn.Stmt(statement.InsertIntoShiftTable)
	_, err = stmt.Exec(sC.TimestampMs, sC.TimestampMsEnd, AssetTableID, 0)
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
