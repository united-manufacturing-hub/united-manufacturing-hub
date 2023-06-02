// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

type ModifyState struct{}

type modifyState struct {
	StartTimeStampMs *uint32 `json:"timestamp_ms"`
	EndTimeStampMs   *uint32 `json:"timestamp_ms_end"`
	NewState         *uint32 `json:"new_state"`
}

// ProcessMessages processes a ModifyState kafka message, by creating an database connection, decoding the json payload, retrieving the required additional database id's (like AssetTableID or ProductTableID) and then inserting it into the database and committing
func (c ModifyState) ProcessMessages(msg internal.ParsedMessage) (putback bool, err error, forcePbTopic bool) {

	txnCtx, txnCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnCtxCl is the cancel function of the context, used in the transaction creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnCtxCl()
	var txn *sql.Tx
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

	// sC is the payload, parsed as modifyState
	var sC modifyState
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		zap.S().Warnf("Failed to unmarshal message: %s", err.Error())
		return false, err, true
	}
	if !internal.IsValidStruct(sC, []string{}) {
		zap.S().Warnf("Invalid message: %s, inserting into putback !", string(msg.Payload))
		return true, nil, true
	}

	AssetTableID, success := GetAssetTableID(msg.CustomerId, msg.Location, msg.AssetId)
	if !success {
		zap.S().Warnf("Failed to get AssetTableID")
		return true, fmt.Errorf(
			"failed to get AssetTableID for CustomerId: %s, Location: %s, AssetId: %s",
			msg.CustomerId,
			msg.Location,
			msg.AssetId), false
	}

	// Changes should only be necessary between this marker

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnStmtCtxCl is the cancel function of the context, used in the statement creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnStmtCtxCl()

	stmtGetLastInRange := txn.StmtContext(txnStmtCtx, statement.SelectLastStateFromStateTableInRange)
	stmtDeleteInRange := txn.StmtContext(txnStmtCtx, statement.DeleteFromStateTableByTimestampRangeAndAssetId)
	stmtInsertNewState := txn.StmtContext(txnStmtCtx, statement.InsertIntoStateTable)
	stmtDeleteOldState := txn.StmtContext(txnStmtCtx, statement.DeleteFromStateTableByTimestamp)

	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// stmtCtxCl is the cancel function of the context, used in the transactions execution creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer stmtCtxCl()

	val, err := stmtGetLastInRange.QueryContext(stmtCtx, sC.StartTimeStampMs, AssetTableID)
	if err != nil {
		zap.S().Debugf("Error inserting into count table: %s", err.Error())
		return true, err, false
	}

	if val.Next() {
		var (
			LastRowTimestamp    float64
			LastRowTimestampInt int64
			LastRowAssetId      int64
			LastRowState        int64
		)
		err = val.Scan(&LastRowTimestamp, &LastRowAssetId, &LastRowState)
		if err != nil {
			zap.S().Debugf("Error scanning val: %s", err.Error())
			return true, err, false
		}
		LastRowTimestampInt = int64(LastRowTimestamp)
		err = val.Close()
		if err != nil {
			zap.S().Debugf("Error closing val: %s", err.Error())
			return true, err, false
		}

		_, err = stmtDeleteInRange.ExecContext(stmtCtx, sC.StartTimeStampMs, sC.EndTimeStampMs, AssetTableID)
		if err != nil {
			zap.S().Debugf("Error deleting state: %s", err.Error())
			return true, err, false
		}
		_, err = stmtInsertNewState.ExecContext(stmtCtx, sC.StartTimeStampMs, AssetTableID, sC.NewState)
		if err != nil {
			zap.S().Debugf("Error inserting new state: %s", err.Error())
			return true, err, false
		}
		_, err = stmtDeleteOldState.ExecContext(stmtCtx, LastRowTimestampInt)
		if err != nil {
			zap.S().Debugf("Error deleting old state: %s", err.Error())
			return true, err, false
		}
		_, err = stmtInsertNewState.ExecContext(stmtCtx, sC.EndTimeStampMs, AssetTableID, LastRowState)
		if err != nil {
			zap.S().Debugf("Error re-inserting state: %s", err.Error())
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

	zap.S().Debugf("Successfully processed modifyState message: %v", msg)
	return false, err, false
}
