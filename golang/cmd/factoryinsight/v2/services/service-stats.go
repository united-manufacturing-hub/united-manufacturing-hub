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

package services

import (
	"database/sql"
	"errors"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
)

// GetDatabaseStatistics returns the database statistics
func GetDatabaseStatistics() (datamodel.DatabaseStatistics, error) {
	var dbStats datamodel.DatabaseStatistics
	dbStats.DatabaseSizeInBytes = GetDatabaseSizeInBytes()
	tableNames := GetAllTableNamesInPublic()
	dbStats.TableStatistics = make(map[string]datamodel.DatabaseTableStatistics, len(tableNames))
	for _, tableName := range tableNames {
		var dbTableStats datamodel.DatabaseTableStatistics
		dbTableStats.IsHyperTable = IsTableHyperTable(tableName)
		dbTableStats.LastAutoAnalyze, dbTableStats.LastAnalyze, dbTableStats.LastVacuum, dbTableStats.LastAutoVacuum = GetVacuumAndAnalyzeStats(tableName)
		dbTableStats.NormalStats = GetNormalTableStatistics(tableName)
		if dbTableStats.IsHyperTable {
			dbTableStats.HyperStats = GetHypertableStats(tableName)
			dbTableStats.HyperRetention = GetHyperRetentionSettings(tableName)
			dbTableStats.HyperCompression = GetHyperCompressionSettings(tableName)
			dbTableStats.ApproximateRows = GetApproximateHyperRows(tableName)
		} else {
			dbTableStats.ApproximateRows = GetApproximateNormalRows(tableName)
		}

		dbStats.TableStatistics[tableName] = dbTableStats
	}
	return dbStats, nil
}

func GetApproximateHyperRows(tableName string) (approximateRows int64) {
	sqlStatement := `
				-- noinspection SqlResolve
	SELECT approximate_row_count FROM approximate_row_count($1);`

	err := database.Db.QueryRow(sqlStatement, tableName).Scan(&approximateRows)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return approximateRows
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return approximateRows
	}
	return approximateRows
}

func GetApproximateNormalRows(tableName string) (approximateRows int64) {
	//https://stackoverflow.com/questions/7943233/fast-way-to-discover-the-row-count-of-a-table-in-postgresql
	sqlStatement := `-- noinspection SqlResolveForFile @ any/"pg_catalog"
	
		SELECT (CASE WHEN c.reltuples < 0 THEN NULL       -- never vacuumed	
	WHEN c.relpages = 0 THEN float8 '0'  -- empty table
	ELSE c.reltuples / c.relpages END
	* (pg_catalog.pg_relation_size(c.oid)	
	  / pg_catalog.current_setting('block_size')::int)
		  )::bigint
	FROM   pg_catalog.pg_class c
	WHERE  c.oid = $1::regclass;      -- schema-qualified table here
		`
	err := database.Db.QueryRow(sqlStatement, tableName).Scan(&approximateRows)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return approximateRows
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return approximateRows
	}
	return approximateRows
}

func GetNormalTableStatistics(tableName string) datamodel.DatabaseNormalTableStatistics {
	// using pg_table_size, pg_total_relation_size, pg_indexes_size, pg_relation_size (main, fsm, vm, init)
	sqlStatement := `
		SELECT pg_table_size($1),
		       pg_total_relation_size($1),
		       pg_indexes_size($1),
		       pg_relation_size($1, 'main'),
		       pg_relation_size($1, 'fsm'),
		       pg_relation_size($1, 'vm'),
		       pg_relation_size($1, 'init')
	`

	var dbNormalTableStats datamodel.DatabaseNormalTableStatistics
	err := database.Db.QueryRow(sqlStatement, tableName).Scan(&dbNormalTableStats.PgTableSize, &dbNormalTableStats.PgTotalRelationSize, &dbNormalTableStats.PgIndexesSize, &dbNormalTableStats.PgRelationSizeMain, &dbNormalTableStats.PgRelationSizeFsm, &dbNormalTableStats.PgRelationSizeVm, &dbNormalTableStats.PgRelationSizeInit)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return dbNormalTableStats
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return dbNormalTableStats
	}
	return dbNormalTableStats

}

func IsTableHyperTable(tableName string) (isHyerTable bool) {
	// SELECT COUNT(*) FROM timescaledb_information.hypertables WHERE hypertable_name = 'processvaluetable';
	sqlStatement := `-- noinspection SqlResolveForFile @ any/"timescaledb_information"
	
			SELECT COUNT(*) FROM timescaledb_information.hypertables WHERE hypertable_name = $1;
		`

	var count int
	err := database.Db.QueryRow(sqlStatement, tableName).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return false
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return false
	}
	return count > 0
}

func GetHypertableStats(tableName string) (hypertableStats []datamodel.DatabaseHyperTableStatistics) {
	hypertableStats = make([]datamodel.DatabaseHyperTableStatistics, 0)
	sqlStatement := `-- noinspection SqlResolveForFile @ routine/"hypertable_detailed_size"
	
			 SELECT * FROM hypertable_detailed_size($1);
		`
	rows, err := database.Db.Query(sqlStatement, tableName)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return hypertableStats
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return hypertableStats
	}
	defer rows.Close()

	for rows.Next() {
		var hypertableStat datamodel.DatabaseHyperTableStatistics
		err = rows.Scan(&hypertableStat.TableBytes, &hypertableStat.IndexBytes, &hypertableStat.ToastBytes, &hypertableStat.TotalBytes, &hypertableStat.NodeName)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return hypertableStats
		}
		hypertableStats = append(hypertableStats, hypertableStat)
	}
	return hypertableStats
}

func GetHyperRetentionSettings(tableName string) (retentionSettings datamodel.DatabaseHyperTableRetention) {
	sqlStatement := `-- noinspection SqlResolveForFile @ any/"timescaledb_information"
	
	SELECT schedule_interval, config FROM timescaledb_information.jobs
	WHERE hypertable_name = $1
	AND timescaledb_information.jobs.proc_name = 'policy_retention';
	`
	err := database.Db.QueryRow(sqlStatement, tableName).Scan(&retentionSettings.ScheduleInterval, &retentionSettings.Config)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return retentionSettings
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return retentionSettings
	}

	return retentionSettings
}
func GetHyperCompressionSettings(tableName string) (retentionSettings datamodel.DatabaseHyperTableCompression) {
	sqlStatement := `-- noinspection SqlResolveForFile @ any/"timescaledb_information"
	
	SELECT schedule_interval, config FROM timescaledb_information.jobs
	WHERE hypertable_name = $1
	AND timescaledb_information.jobs.proc_name = 'policy_compression';
	`
	err := database.Db.QueryRow(sqlStatement, tableName).Scan(&retentionSettings.ScheduleInterval, &retentionSettings.Config)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return retentionSettings
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return retentionSettings
	}

	return retentionSettings
}

func GetAllTableNamesInPublic() (tableNames []string) {
	tableNames = make([]string, 0)
	sqlStatement := `-- noinspection SqlResolveForFile @ any/"information_schema"

			SELECT table_name FROM information_schema.tables WHERE table_schema='public';
		`
	rows, err := database.Db.Query(sqlStatement)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return tableNames
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return tableNames
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		err = rows.Scan(&tableName)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return tableNames
		}
		tableNames = append(tableNames, tableName)
	}
	return tableNames
}

func GetDatabaseSizeInBytes() (size int64) {
	sqlStatement := `
	SELECT pg_database_size('factoryinsight');`

	err := database.Db.QueryRow(sqlStatement).Scan(&size)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return 0
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return 0
	}

	return size
}

func GetVacuumAndAnalyzeStats(tableName string) (lastAutoanalyze, lastAnalyze, lastVacuum, lastAutovacuum sql.NullString) {
	sqlStatement := `-- noinspection SqlResolveForFile @ table/"pg_stat_all_tables"

		SELECT schemaname, relname, last_autoanalyze, last_analyze, last_vacuum, last_autovacuum FROM pg_stat_all_tables WHERE relname = $1;`

	var schemaName, relName string
	err := database.Db.QueryRow(sqlStatement, tableName).Scan(&schemaName, &relName, &lastAutoanalyze, &lastAnalyze, &lastVacuum, &lastAutovacuum)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return lastAutoanalyze, lastAnalyze, lastVacuum, lastAutovacuum
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return lastAutoanalyze, lastAnalyze, lastVacuum, lastAutovacuum

	}

	return lastAutoanalyze, lastAnalyze, lastVacuum, lastAutovacuum
}
