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

import (
	"database/sql"
	"go.uber.org/zap"
)

type StatementRegistry struct {
	InsertIntoRecommendationTable *sql.Stmt

	CreateTmpProcessValueTable64 *sql.Stmt

	CreateTmpProcessValueTable *sql.Stmt

	CreateTmpCountTable *sql.Stmt

	InsertIntoStateTable *sql.Stmt

	UpdateCountTableScrap *sql.Stmt

	InsertIntoUniqueProductTable *sql.Stmt

	InsertIntoProductTagTable *sql.Stmt

	InsertIntoProductTagStringTable *sql.Stmt

	InsertIntoProductInheritanceTable *sql.Stmt

	InsertIntoShiftTable *sql.Stmt

	UpdateUniqueProductTableSetIsScrap *sql.Stmt

	InsertIntoProductTable *sql.Stmt

	InsertIntoOrderTable *sql.Stmt

	UpdateOrderTableSetBeginTimestamp *sql.Stmt

	UpdateOrderTableSetEndTimestamp *sql.Stmt

	InsertIntoMaintenanceActivities *sql.Stmt

	SelectLastStateFromStateTableInRange *sql.Stmt

	DeleteFromStateTableByTimestampRangeAndAssetId *sql.Stmt

	DeleteFromStateTableByTimestamp *sql.Stmt

	DeleteFromShiftTableByAssetIDAndBeginTimestamp *sql.Stmt

	InsertIntoAssetTable *sql.Stmt

	UpdateCountTableSetCountAndScrapByAssetIdAndTs *sql.Stmt

	UpdateCountTableSetCountByAssetIdAndTs *sql.Stmt
	UpdateCountTableSetScrapByAssetIdAndTs *sql.Stmt

	SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId *sql.Stmt

	SelectProductIdFromProductTableByAssetIdAndProductName *sql.Stmt

	SelectIdFromComponentTableByAssetIdAndComponentName *sql.Stmt

	SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndAssetIdOrderedByTimeStampDesc *sql.Stmt

	SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndNotAssetId *sql.Stmt
	CreateTmpProcessValueTableString                                                     *sql.Stmt
	SelectProductExists                                                                  *sql.Stmt

	InsertIntoCountTable *sql.Stmt
	SelectOrderExists    *sql.Stmt
}

func (r StatementRegistry) Shutdown() {
	zap.S().Infof("[statementRegistry] shutting down!")
	_ = r.InsertIntoRecommendationTable.Close()

	_ = r.InsertIntoRecommendationTable.Close()

	_ = r.CreateTmpProcessValueTable64.Close()

	_ = r.CreateTmpProcessValueTable.Close()

	_ = r.CreateTmpCountTable.Close()

	_ = r.InsertIntoStateTable.Close()

	_ = r.UpdateCountTableScrap.Close()

	_ = r.InsertIntoUniqueProductTable.Close()

	_ = r.InsertIntoProductTagTable.Close()

	_ = r.InsertIntoProductTagStringTable.Close()

	_ = r.InsertIntoProductInheritanceTable.Close()

	_ = r.InsertIntoShiftTable.Close()

	_ = r.UpdateUniqueProductTableSetIsScrap.Close()

	_ = r.InsertIntoProductTable.Close()

	_ = r.InsertIntoOrderTable.Close()

	_ = r.UpdateOrderTableSetBeginTimestamp.Close()

	_ = r.UpdateOrderTableSetEndTimestamp.Close()

	_ = r.InsertIntoMaintenanceActivities.Close()

	_ = r.SelectLastStateFromStateTableInRange.Close()

	_ = r.DeleteFromStateTableByTimestampRangeAndAssetId.Close()

	_ = r.DeleteFromStateTableByTimestamp.Close()

	_ = r.DeleteFromShiftTableByAssetIDAndBeginTimestamp.Close()

	_ = r.InsertIntoAssetTable.Close()

	_ = r.UpdateCountTableSetCountAndScrapByAssetIdAndTs.Close()

	_ = r.UpdateCountTableSetCountByAssetIdAndTs.Close()

	_ = r.UpdateCountTableSetScrapByAssetIdAndTs.Close()

	_ = r.SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId.Close()

	_ = r.SelectProductIdFromProductTableByAssetIdAndProductName.Close()

	_ = r.SelectIdFromComponentTableByAssetIdAndComponentName.Close()

	_ = r.SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndAssetIdOrderedByTimeStampDesc.Close()

	_ = r.SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndNotAssetId.Close()

	_ = r.CreateTmpProcessValueTableString.Close()

	_ = r.SelectProductExists.Close()

	_ = r.InsertIntoCountTable.Close()

	_ = r.SelectOrderExists.Close()
}

func NewStatementRegistry() *StatementRegistry {

	return &StatementRegistry{
		InsertIntoRecommendationTable: prep(
			`
		INSERT INTO recommendationTable (timestamp, uid, recommendationType, enabled, recommendationValues, recommendationTextEN, recommendationTextDE, diagnoseTextEN, diagnoseTextDE) 
		VALUES (to_timestamp($1 / 1000.0),$2,$3,$4,$5,$6,$7,$8,$9) 
		ON CONFLICT (uid) DO UPDATE 
		SET timestamp=to_timestamp($1 / 1000.0), uid=$2, recommendationType=$3, enabled=$4, recommendationValues=$5, recommendationTextEN=$6, recommendationTextDE=$7, diagnoseTextEN=$8, diagnoseTextDE=$9;`),

		CreateTmpProcessValueTable64: prep(
			`
			CREATE TEMP TABLE tmp_processvaluetable64 
				( LIKE processValueTable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`),

		CreateTmpProcessValueTable: prep(
			`
			CREATE TEMP TABLE tmp_processvaluetable 
				( LIKE processValueTable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`),

		CreateTmpCountTable: prep(
			`
			CREATE TEMP TABLE tmp_counttable
				( LIKE counttable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`),

		CreateTmpProcessValueTableString: prep(
			`
			CREATE TEMP TABLE tmp_processvaluestringtable 
				( LIKE processValueStringTable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`),

		InsertIntoStateTable: prep(
			`
		INSERT INTO statetable (timestamp, asset_id, state) 
		VALUES (to_timestamp($1 / 1000.0),$2,$3)`),

		UpdateCountTableScrap: prep(
			`
		UPDATE counttable 
		SET scrap = count 
		WHERE (timestamp, asset_id) IN
			(SELECT timestamp, asset_id
			FROM (
				SELECT *, sum(count) OVER (ORDER BY timestamp DESC) AS running_total
				FROM countTable
				WHERE timestamp < to_timestamp($1/1000.0) AND timestamp > (to_timestamp($1/1000.0)::TIMESTAMP - INTERVAL '1 DAY') AND asset_id = $2
			) t
			WHERE running_total <= $3)
		;`),

		InsertIntoUniqueProductTable: prep(
			`
		INSERT INTO uniqueProductTable (asset_id, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap, uniqueproductalternativeid) 
		VALUES ($1, to_timestamp($2 / 1000.0),to_timestamp($3 / 1000.0),$4,$5,$6) 
		ON CONFLICT DO NOTHING;`),

		InsertIntoProductTagTable: prep(
			`
		INSERT INTO productTagTable (valueName, value, timestamp, product_uid) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0), $4) 
		ON CONFLICT DO NOTHING;`),

		InsertIntoProductTagStringTable: prep(
			`
		INSERT INTO productTagStringTable (valueName, value, timestamp, product_uid) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0), $4) 
		ON CONFLICT DO NOTHING;`),

		InsertIntoProductInheritanceTable: prep(
			`
		INSERT INTO productInheritanceTable (parent_uid, child_uid, timestamp) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0)) 
		ON CONFLICT DO NOTHING;`),

		InsertIntoShiftTable: prep(
			`
		INSERT INTO shiftTable (begin_timestamp, end_timestamp, asset_id, type) 
		VALUES (to_timestamp($1 / 1000.0),to_timestamp($2 / 1000.0),$3,$4) 
		ON CONFLICT (begin_timestamp, asset_id) DO UPDATE 
		SET begin_timestamp=to_timestamp($1 / 1000.0), end_timestamp=to_timestamp($2 / 1000.0), asset_id=$3, type=$4;`),

		UpdateUniqueProductTableSetIsScrap: prep(`UPDATE uniqueProductTable SET is_scrap = True WHERE uniqueProductID = $1 AND asset_id = $2;`),

		InsertIntoProductTable: prep(
			`INSERT INTO productTable (asset_id, product_name, time_per_unit_in_seconds)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING;`),

		InsertIntoOrderTable: prep(
			`INSERT INTO orderTable (order_name, product_id, target_units, asset_id) 
		VALUES ($1, $2, $3, $4) 
		ON CONFLICT DO NOTHING;`),

		UpdateOrderTableSetBeginTimestamp: prep(
			`UPDATE orderTable 
		SET begin_timestamp = to_timestamp($1 / 1000.0) 
		WHERE order_name=$2 
			AND asset_id = $3;`),

		UpdateOrderTableSetEndTimestamp: prep(
			`UPDATE orderTable 
		SET end_timestamp = to_timestamp($1 / 1000.0) 
		WHERE order_name=$2 
			AND asset_id = $3;`),

		SelectOrderExists: prep(`SELECT count(1) FROM orderTable WHERE order_name=$1 AND asset_id=$2;`),

		InsertIntoMaintenanceActivities: prep(
			`INSERT INTO maintenanceactivities (component_id, activitytype, timestamp) 
	VALUES ($1, $2, to_timestamp($3 / 1000.0)) 
	ON CONFLICT DO NOTHING;`),

		SelectLastStateFromStateTableInRange: prep(`SELECT extract(epoch from timestamp)*1000, asset_id, state FROM statetable WHERE timestamp > to_timestamp($1 / 1000.0) AND asset_id = $2 ORDER BY timestamp ASC LIMIT 1;`),

		DeleteFromStateTableByTimestampRangeAndAssetId: prep(`DELETE FROM statetable WHERE timestamp >= to_timestamp($1 / 1000.0) AND timestamp <= to_timestamp($2 / 1000.0) AND asset_id = $3;`),

		DeleteFromStateTableByTimestamp: prep(`DELETE FROM statetable WHERE timestamp = to_timestamp($1 / 1000.0);`),

		DeleteFromShiftTableByAssetIDAndBeginTimestamp: prep(`DELETE FROM shifttable WHERE asset_id = $1 AND begin_timestamp = to_timestamp($2 / 1000.0);`),

		InsertIntoAssetTable: prep(
			`
		INSERT INTO assetTable(assetID, location, customer) 
		VALUES ($1,$2,$3) 
		ON CONFLICT DO NOTHING;`),

		UpdateCountTableSetCountAndScrapByAssetIdAndTs: prep(`UPDATE counttable SET count = $1, scrap = $2 WHERE asset_id = $3 AND timestamp = to_timestamp($4 / 1000.0);`),

		UpdateCountTableSetCountByAssetIdAndTs: prep(`UPDATE counttable SET count = $1 WHERE asset_id = $2 AND timestamp = to_timestamp($3 / 1000.0);`),

		UpdateCountTableSetScrapByAssetIdAndTs: prep(`UPDATE counttable SET scrap = $1 WHERE asset_id = $2 AND timestamp = to_timestamp($3 / 1000.0);`),

		SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId: prep(`SELECT id FROM assetTable WHERE assetid=$1 AND location=$2 AND customer=$3;`),

		SelectProductIdFromProductTableByAssetIdAndProductName: prep(`SELECT product_id FROM productTable WHERE asset_id=$1 AND product_name=$2;`),

		SelectIdFromComponentTableByAssetIdAndComponentName: prep(`SELECT id FROM componentTable WHERE asset_id=$1 AND componentName=$2;`),

		SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndAssetIdOrderedByTimeStampDesc: prep(`SELECT uniqueProductID FROM uniqueProductTable WHERE uniqueProductAlternativeID = $1 AND asset_id = $2 ORDER BY begin_timestamp_ms DESC LIMIT 1;`),

		SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndNotAssetId: prep(`SELECT uniqueProductID FROM uniqueProductTable WHERE uniqueProductAlternativeID = $1 AND NOT asset_id = $2 ORDER BY begin_timestamp_ms DESC LIMIT 1;`),

		SelectProductExists: prep(`SELECT COUNT(*) as CNT FROM producttable WHERE product_id = $1`),

		InsertIntoCountTable: prep(`INSERT INTO counttable (asset_id, count, scrap, timestamp) VALUES ($1,$2,$3,to_timestamp($4 / 1000.0)) ON CONFLICT DO NOTHING`),
	}
}

func prep(query string) *sql.Stmt {
	if db == nil {
		zap.S().Fatal("db is nil")
	}
	prepare, err := db.Prepare(query)
	if err != nil {
		zap.S().Fatalf("Failed to prepare statement: %s (%s)", query, err.Error)
	}
	return prepare
}
