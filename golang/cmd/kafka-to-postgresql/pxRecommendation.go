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
	"errors"
	jsoniter "github.com/json-iterator/go"
	"github.com/lib/pq"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"time"
)

type Recommendation struct{}

type recommendation struct {
	UID                  *string
	TimestampMs          *uint64 `json:"timestamp_ms"`
	Customer             *string
	Location             *string
	Asset                *string
	RecommendationType   *int32
	Enabled              *bool
	RecommendationValues *string
	DiagnoseTextDE       *string
	DiagnoseTextEN       *string
	RecommendationTextDE *string
	RecommendationTextEN *string
}

// ProcessMessages processes a Recommendation kafka message, by creating an database connection, decoding the json payload, retrieving the required additional database id's (like AssetTableID or ProductTableID) and then inserting it into the database and committing
func (c Recommendation) ProcessMessages(msg internal.ParsedMessage) (putback bool, err error, forcePbTopic bool) {

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

	// sC is the payload, parsed as recommendation
	var sC recommendation
	err = jsoniter.Unmarshal(msg.Payload, &sC)
	if err != nil {
		zap.S().Warnf("Failed to unmarshal message: %s", err.Error())
		return false, err, true
	}
	if !internal.IsValidStruct(sC, []string{}) {
		zap.S().Warnf("Invalid message: %s, inserting into putback !", string(msg.Payload))
		return true, nil, true
	}

	// Changes should only be necessary between this marker

	txnStmtCtx, txnStmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// txnStmtCtxCl is the cancel function of the context, used in the statement creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer txnStmtCtxCl()

	stmt := txn.StmtContext(txnStmtCtx, statement.InsertIntoRecommendationTable)

	stmtCtx, stmtCtxCl := context.WithDeadline(context.Background(), time.Now().Add(internal.FiveSeconds))
	// stmtCtxCl is the cancel function of the context, used in the transactions execution creation.
	// It is deferred to automatically release the allocated resources, once the function returns
	defer stmtCtxCl()

	_, err = stmt.ExecContext(
		stmtCtx,
		sC.UID,
		sC.RecommendationType,
		sC.Enabled,
		sC.RecommendationValues,
		sC.RecommendationTextEN,
		sC.RecommendationTextDE,
		sC.DiagnoseTextEN,
		sC.DiagnoseTextDE)
	if err != nil {

		if err != nil {
			var pqErr *pq.Error
			ok := errors.As(err, &pqErr)

			if !ok {
				zap.S().Errorf("Failed to convert error to pq.Error: %s", err.Error())
				return true, err, false
			} else {
				zap.S().Errorf("Error executing statement: %s -> %s", pqErr.Code, pqErr.Message)
				if pqErr.Code == Sql23p01ExclusionViolation {
					return true, err, true
				} else if pqErr.Code == Sql23505UniqueViolation {
					return true, err, true
				}
			}
			return true, err, false
		}
		zap.S().Debugf("Error inserting into recommendation table: %s", err.Error())
		return true, err, false
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

	zap.S().Debugf("Successfully processed recommendation message: %v", msg)
	return false, err, false
}
