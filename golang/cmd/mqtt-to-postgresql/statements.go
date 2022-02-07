package main

import (
	"database/sql"
	"go.uber.org/zap"
	"time"
)

type statementRegistry struct {
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

	DeleteFromShiftTableById *sql.Stmt

	DeleteFromShiftTableByAssetIDAndBeginTimestamp *sql.Stmt

	InsertIntoAssetTable *sql.Stmt

	UpdateCountTableSetCountAndScrapByAssetId *sql.Stmt

	UpdateCountTableSetCountByAssetId *sql.Stmt
	UpdateCountTableSetScrapByAssetId *sql.Stmt

	SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId *sql.Stmt

	SelectProductIdFromProductTableByAssetIdAndProductName *sql.Stmt

	SelectIdFromComponentTableByAssetIdAndComponentName *sql.Stmt

	SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndAssetIdOrderedByTimeStampDesc *sql.Stmt

	SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndNotAssetId *sql.Stmt
	CreateTmpProcessValueTableString                                                     *sql.Stmt
	SelectProductExists                                                                  *sql.Stmt
}

func (r statementRegistry) Shutdown() (err error) {
	zap.S().Warnf("[statementRegistry] shutting down!")
	err = r.InsertIntoRecommendationTable.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoRecommendationTable.Close()
	if err != nil {
		return
	}
	err = r.CreateTmpProcessValueTable64.Close()
	if err != nil {
		return
	}
	err = r.CreateTmpProcessValueTable.Close()
	if err != nil {
		return
	}
	err = r.CreateTmpCountTable.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoStateTable.Close()
	if err != nil {
		return
	}
	err = r.UpdateCountTableScrap.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoUniqueProductTable.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoProductTagTable.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoProductTagStringTable.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoProductInheritanceTable.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoShiftTable.Close()
	if err != nil {
		return
	}
	err = r.UpdateUniqueProductTableSetIsScrap.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoProductTable.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoOrderTable.Close()
	if err != nil {
		return
	}
	err = r.UpdateOrderTableSetBeginTimestamp.Close()
	if err != nil {
		return
	}
	err = r.UpdateOrderTableSetEndTimestamp.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoMaintenanceActivities.Close()
	if err != nil {
		return
	}
	err = r.SelectLastStateFromStateTableInRange.Close()
	if err != nil {
		return
	}
	err = r.DeleteFromStateTableByTimestampRangeAndAssetId.Close()
	if err != nil {
		return
	}
	err = r.DeleteFromStateTableByTimestamp.Close()
	if err != nil {
		return
	}
	err = r.DeleteFromShiftTableById.Close()
	if err != nil {
		return
	}
	err = r.DeleteFromShiftTableByAssetIDAndBeginTimestamp.Close()
	if err != nil {
		return
	}
	err = r.InsertIntoAssetTable.Close()
	if err != nil {
		return
	}
	err = r.UpdateCountTableSetCountAndScrapByAssetId.Close()
	if err != nil {
		return
	}
	err = r.UpdateCountTableSetCountByAssetId.Close()
	if err != nil {
		return
	}
	err = r.UpdateCountTableSetScrapByAssetId.Close()
	if err != nil {
		return
	}
	err = r.SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId.Close()
	if err != nil {
		return
	}
	err = r.SelectProductIdFromProductTableByAssetIdAndProductName.Close()
	if err != nil {
		return
	}
	err = r.SelectIdFromComponentTableByAssetIdAndComponentName.Close()
	if err != nil {
		return
	}
	err = r.SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndAssetIdOrderedByTimeStampDesc.Close()
	if err != nil {
		return
	}
	err = r.SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndNotAssetId.Close()
	if err != nil {
		return
	}
	err = r.CreateTmpProcessValueTableString.Close()
	if err != nil {
		return
	}
	err = r.SelectProductExists.Close()
	if err != nil {
		return
	}
	return
}

func newStatementRegistry() *statementRegistry {

	return &statementRegistry{
		InsertIntoRecommendationTable: prep(`
		INSERT INTO recommendationTable (timestamp, uid, recommendationType, enabled, recommendationValues, recommendationTextEN, recommendationTextDE, diagnoseTextEN, diagnoseTextDE) 
		VALUES (to_timestamp($1 / 1000.0),$2,$3,$4,$5,$6,$7,$8,$9) 
		ON CONFLICT (uid) DO UPDATE 
		SET timestamp=to_timestamp($1 / 1000.0), uid=$2, recommendationType=$3, enabled=$4, recommendationValues=$5, recommendationTextEN=$6, recommendationTextDE=$7, diagnoseTextEN=$8, diagnoseTextDE=$9;`, 0),

		CreateTmpProcessValueTable64: prep(`
			CREATE TEMP TABLE tmp_processvaluetable64 
				( LIKE processValueTable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`, 0),

		CreateTmpProcessValueTable: prep(`
			CREATE TEMP TABLE tmp_processvaluetable 
				( LIKE processValueTable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`, 0),

		CreateTmpCountTable: prep(`
			CREATE TEMP TABLE tmp_counttable
				( LIKE counttable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`, 0),

		CreateTmpProcessValueTableString: prep(`
			CREATE TEMP TABLE tmp_processvaluestringtable 
				( LIKE processValueStringTable INCLUDING DEFAULTS ) ON COMMIT DROP 
			;
		`, 0),

		InsertIntoStateTable: prep(`
		INSERT INTO statetable (timestamp, asset_id, state) 
		VALUES (to_timestamp($1 / 1000.0),$2,$3)`, 0),

		UpdateCountTableScrap: prep(`
		UPDATE counttable 
		SET scrap = count 
		WHERE (timestamp, asset_id) IN
			(SELECT timestamp, asset_id
			FROM (
				SELECT *, sum(count) OVER (ORDER BY timestamp DESC) AS running_total
				FROM countTable
				WHERE timestamp < to_timestamp($1/1000) AND timestamp > (to_timestamp($1/1000)::TIMESTAMP - INTERVAL '1 DAY') AND asset_id = $2
			) t
			WHERE running_total <= $3)
		;`, 0),

		InsertIntoUniqueProductTable: prep(`
		INSERT INTO uniqueProductTable (asset_id, begin_timestamp_ms, end_timestamp_ms, product_id, is_scrap, uniqueproductalternativeid) 
		VALUES ($1, to_timestamp($2 / 1000.0),to_timestamp($3 / 1000.0),$4,$5,$6) 
		ON CONFLICT DO NOTHING;`, 0),

		InsertIntoProductTagTable: prep(`
		INSERT INTO productTagTable (valueName, value, timestamp, product_uid) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0), $4) 
		ON CONFLICT DO NOTHING;`, 0),

		InsertIntoProductTagStringTable: prep(`
		INSERT INTO productTagStringTable (valueName, value, timestamp, product_uid) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0), $4) 
		ON CONFLICT DO NOTHING;`, 0),

		InsertIntoProductInheritanceTable: prep(`
		INSERT INTO productInheritanceTable (parent_uid, child_uid, timestamp) 
		VALUES ($1, $2, to_timestamp($3 / 1000.0)) 
		ON CONFLICT DO NOTHING;`, 0),

		InsertIntoShiftTable: prep(`
		INSERT INTO shiftTable (begin_timestamp, end_timestamp, asset_id, type) 
		VALUES (to_timestamp($1 / 1000.0),to_timestamp($2 / 1000.0),$3,$4) 
		ON CONFLICT (begin_timestamp, asset_id) DO UPDATE 
		SET begin_timestamp=to_timestamp($1 / 1000.0), end_timestamp=to_timestamp($2 / 1000.0), asset_id=$3, type=$4;`, 0),

		UpdateUniqueProductTableSetIsScrap: prep(`UPDATE uniqueProductTable SET is_scrap = True WHERE uniqueProductID = $1 AND asset_id = $2;`, 0),

		InsertIntoProductTable: prep(`INSERT INTO productTable (asset_id, product_name, time_per_unit_in_seconds)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING;`, 0),

		InsertIntoOrderTable: prep(`INSERT INTO orderTable (order_name, product_id, target_units, asset_id) 
		VALUES ($1, $2, $3, $4) 
		ON CONFLICT DO NOTHING;`, 0),

		UpdateOrderTableSetBeginTimestamp: prep(`UPDATE orderTable 
		SET begin_timestamp = to_timestamp($1 / 1000.0) 
		WHERE order_name=$2 
			AND asset_id = $3;`, 0),

		UpdateOrderTableSetEndTimestamp: prep(`UPDATE orderTable 
		SET end_timestamp = to_timestamp($1 / 1000.0) 
		WHERE order_name=$2 
			AND asset_id = $3;`, 0),

		InsertIntoMaintenanceActivities: prep(`INSERT INTO maintenanceactivities (component_id, activitytype, timestamp) 
	VALUES ($1, $2, to_timestamp($3 / 1000.0)) 
	ON CONFLICT DO NOTHING;`, 0),

		SelectLastStateFromStateTableInRange: prep(`SELECT extract(epoch from timestamp)*1000, asset_id, state FROM statetable WHERE timestamp > to_timestamp($1 / 1000.0) AND asset_id = $2 ORDER BY timestamp ASC LIMIT 1;`, 0),

		DeleteFromStateTableByTimestampRangeAndAssetId: prep(`DELETE FROM statetable WHERE timestamp >= to_timestamp($1 / 1000.0) AND timestamp <= to_timestamp($2 / 1000.0) AND asset_id = $3;`, 0),

		DeleteFromStateTableByTimestamp: prep(`DELETE FROM statetable WHERE timestamp = to_timestamp($1 / 1000.0);`, 0),

		DeleteFromShiftTableById: prep(`DELETE FROM shifttable WHERE id = $1;`, 0),

		DeleteFromShiftTableByAssetIDAndBeginTimestamp: prep(`DELETE FROM shifttable WHERE asset_id = $1 AND begin_timestamp = to_timestamp($2 / 1000.0);`, 0),

		InsertIntoAssetTable: prep(`
		INSERT INTO assetTable(assetID, location, customer) 
		VALUES ($1,$2,$3) 
		ON CONFLICT DO NOTHING;`, 0),

		UpdateCountTableSetCountAndScrapByAssetId: prep(`UPDATE counttable SET count = $1, scrap = $2 WHERE asset_id = $3`, 0),

		UpdateCountTableSetCountByAssetId: prep(`UPDATE counttable SET count = $1 WHERE asset_id = $2`, 0),

		UpdateCountTableSetScrapByAssetId: prep(`UPDATE counttable SET scrap = $1 WHERE asset_id = $2`, 0),

		SelectIdFromAssetTableByAssetIdAndLocationIdAndCustomerId: prep(`SELECT id FROM assetTable WHERE assetid=$1 AND location=$2 AND customer=$3;`, 0),

		SelectProductIdFromProductTableByAssetIdAndProductName: prep(`SELECT product_id FROM productTable WHERE asset_id=$1 AND product_name=$2;`, 0),

		SelectIdFromComponentTableByAssetIdAndComponentName: prep(`SELECT id FROM componentTable WHERE asset_id=$1 AND componentName=$2;`, 0),

		SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndAssetIdOrderedByTimeStampDesc: prep(`SELECT uniqueProductID FROM uniqueProductTable WHERE uniqueProductAlternativeID = $1 AND asset_id = $2 ORDER BY begin_timestamp_ms DESC LIMIT 1;`, 0),

		SelectUniqueProductIdFromUniqueProductTableByUniqueProductAlternativeIdAndNotAssetId: prep(`SELECT uniqueProductID FROM uniqueProductTable WHERE uniqueProductAlternativeID = $1 AND NOT asset_id = $2 ORDER BY begin_timestamp_ms DESC LIMIT 1;`, 0),

		SelectProductExists: prep(`SELECT COUNT(*) as CNT FROM producttable WHERE product_id = $1`, 0),
	}
}

func prep(query string, recursionDepth int) *sql.Stmt {

	if db == nil {
		panic("Attempting to prepare statement before opening database !")
	}
	prepare, err := db.Prepare(query)
	if err != nil {
		switch GetPostgresErrorRecoveryOptions(err) {
		case Unrecoverable:
			ShutdownApplicationGraceful()
		case TryAgain:
			time.Sleep(1 * time.Second)
			return prep(query, recursionDepth+1)
		case DiscardValue:
			// This should NEVER happen here, but it's safer to shut down, else the statement won't be prepared
			ShutdownApplicationGraceful()
		}
		zap.S().Errorf("Failed to prepare statement: %s", query)
		panic(err)
	}
	return prepare
}
