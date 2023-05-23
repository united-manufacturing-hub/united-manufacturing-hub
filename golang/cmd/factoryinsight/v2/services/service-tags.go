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
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/repository"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func GetTagGroups(enterpriseName, siteName, areaName, productionLineName, workCellName string) (
	tagGroups []string,
	err error) {
	zap.S().Infof(
		"[GetTagGroups] Getting tag groups for enterprise %s, site %s, area %s, production line %s and work cell %s",
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
	)

	id, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		return nil, err
	}
	var customTagGroupExists bool
	var customTagGroupStringExists bool
	customTagGroupExists, err = GetCustomTagsExists(id)
	if err != nil {
		return nil, err
	}
	customTagGroupStringExists, err = GetCustomTagsStringExists(id)
	if err != nil {
		return nil, err
	}

	var standardTags []string
	standardTags, err = GetStandardTags(enterpriseName, siteName, workCellName)
	if err != nil {
		return nil, err
	}

	tagGroups = make([]string, 0)
	if customTagGroupExists {
		tagGroups = append(tagGroups, models.CustomTagGroup)
	}
	if customTagGroupStringExists {
		tagGroups = append(tagGroups, models.CustomStringTagGroup)
	}
	if len(standardTags) > 0 {
		tagGroups = append(tagGroups, models.StandardTagGroup)
	}

	return tagGroups, nil
}

func GetStandardTags(enterpriseName, siteName, workCellName string) (tags []string, err error) {
	zap.S().Infof("[GetTags] Getting standard tags")

	tags = make([]string, 0)

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		return nil, err
	}

	jobsExists, err := GetJobsExists(workCellId)
	if err != nil {
		return nil, err
	}
	if jobsExists {
		tags = append(tags, models.JobsStandardTag)
	}

	outputExists, err := GetOutputExists(workCellId)
	if err != nil {
		return nil, err
	}
	if outputExists {
		tags = append(tags, models.OutputStandardTag)
	}

	stateExists, err := GetStateExists(workCellId)
	if err != nil {
		return nil, err
	}
	if stateExists {
		tags = append(tags, models.StateStandardTag)
	}

	shiftExists, err := GetShiftExists(workCellId)
	if err != nil {
		return nil, err
	}
	if shiftExists {
		tags = append(tags, models.ShiftsStandardTag)
	}

	throughputExists, err := GetThroughputExists(workCellId)
	if err != nil {
		return nil, err
	}
	if throughputExists {
		tags = append(tags, models.ThroughputStandardTag)
	}

	return
}

func GetCustomTags(workCellId uint32, isPVS bool) (tags []string, err error) {
	zap.S().Infof(
		"[GetTags] Getting custom tags for work cell %d", workCellId)

	var b bool
	tags, b = GetPrefetchedTags(workCellId, isPVS)
	if b {
		// Retriggers the prefetching of the tags in the background
		go func() {
			err = prefetch(workCellId)
			if err != nil {
				zap.S().Errorf("Error prefetching tags: %s", err)
			}
		}()
		return tags, nil
	}

	tags, err = getCustomTags(workCellId, isPVS)
	if err != nil {
		return nil, err
	}
	updateWorkCellTag(workCellId, tags, isPVS)
	return tags, err
}

type Parent struct {
	Value    string
	Children []Parent
}

func ProcessJobTagRequest(c *gin.Context, request models.GetTagsDataRequest) {

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### parse query ###
	var getJobTagRequest models.GetJobTagRequest

	err = c.BindQuery(&getJobTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// TODO: #97 Return timestamps in RFC3339 in /orderTimeline

	// Process data
	data, err := GetOrdersTimeline(
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
		workCellId,
		getJobTagRequest.From,
		getJobTagRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func ProcessOutputTagRequest(c *gin.Context, request models.GetTagsDataRequest) {

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### parse query ###
	var getCountTagRequest models.GetCountTagRequest
	// var counts ???

	err = c.BindQuery(&getCountTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	// TODO: #88 Return timestamps in RFC3339 in /counts
	counts, err := GetCounts(
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
		workCellId,
		getCountTagRequest.From,
		getCountTagRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

func ProcessShiftsTagRequest(c *gin.Context, request models.GetTagsDataRequest) {

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### parse query ###
	var getShiftsTagRequest models.GetShiftsTagRequest

	err = c.BindQuery(&getShiftsTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	// Fetching from the database
	shifts, err := GetShifts(
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
		workCellId,
		getShiftsTagRequest.From,
		getShiftsTagRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, shifts)
}

// ProcessStateTagRequest is responsible for fetchinf all required data and calculating states over time.
// The result is usually visualized in "DiscretePanel" in Grafana
func ProcessStateTagRequest(c *gin.Context, request models.GetTagsDataRequest) {
	// TODO adapt this to the new data model
	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName
	tagName := request.TagName

	// ### parse query ###
	var getStateTagRequest models.GetStateTagRequest
	var err error

	err = c.BindQuery(&getStateTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getStateTagRequest.From
	to := getStateTagRequest.To
	keepStatesInteger := helpers.StrToBool(getStateTagRequest.KeepStatesInteger)

	// ### fetch necessary data from database ###

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// customer configuration
	configuration, err := GetEnterpriseConfiguration(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	var waitGroup sync.WaitGroup
	waitGroup.Add(4)

	var rawStates []datamodel.StateEntry
	var rawStatesError error

	var rawShifts []datamodel.ShiftEntry
	var rawShiftsError error

	var countSlice []datamodel.CountEntry
	var countSliceError error

	var orderArray []datamodel.OrdersRaw
	var orderArrayError error

	go func() {
		defer waitGroup.Done()
		// raw states from database
		rawStates, rawStatesError = GetStatesRaw(workCellId, from, to, configuration)
	}()

	go func() {
		// get shifts for noShift detection
		defer waitGroup.Done()
		// get shifts for noShift detection
		rawShifts, rawShiftsError = GetShiftsRaw(workCellId, from, to, configuration)
	}()

	go func() {
		// get counts for lowSpeed detection
		defer waitGroup.Done()
		// get counts for lowSpeed detection
		countSlice, countSliceError = GetCountsRaw(workCellId, from, to)
	}()

	// get orders for changeover detection
	go func() {
		// get orders for changeover detection
		defer waitGroup.Done()
		// get orders for changeover detection
		orderArray, orderArrayError = GetOrdersRaw(workCellId, from, to)
	}()

	waitGroup.Wait()

	if rawStatesError != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	if rawShiftsError != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	if countSliceError != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	if orderArrayError != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### calculate (only one function allowed here) ###
	processedStates, err := ProcessStatesOptimized(
		workCellId,
		rawStates,
		rawShifts,
		countSlice,
		orderArray,
		from,
		to,
		configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### create JSON ###
	var data datamodel.DataResponseAny
	// TODO: adapt JSONColumnName to new data model
	JSONColumnName := enterpriseName + "-" + siteName + "-" + areaName + "-" + productionLineName + "-" + workCellName + "-" + tagName
	data.ColumnNames = []string{JSONColumnName, "timestamp"}

	// TODO: #90 Return timestamps in RFC3339 in /state

	// Loop through all datapoints
	for _, dataPoint := range processedStates {
		if keepStatesInteger {
			fullRow := []interface{}{
				dataPoint.State,
				float64(dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
			data.Datapoints = append(data.Datapoints, fullRow)
		} else {
			fullRow := []interface{}{
				repository.ConvertStateToString(dataPoint.State, configuration),
				float64(dataPoint.Timestamp.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))}
			data.Datapoints = append(data.Datapoints, fullRow)
		}
	}

	c.JSON(http.StatusOK, data)
}

func ProcessThroughputTagRequest(c *gin.Context, request models.GetTagsDataRequest) {
	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// ### parse query ###
	var getThroughputTagRequest models.GetThroughputTagRequest
	// var counts ???

	err = c.BindQuery(&getThroughputTagRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	from := getThroughputTagRequest.From
	to := getThroughputTagRequest.To
	aggregationInterval := getThroughputTagRequest.AggregationInterval

	// Fetching from the database
	counts, err := GetProductionSpeed(
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
		workCellId,
		from,
		to,
		aggregationInterval)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

func ProcessCustomTagRequest(c *gin.Context, request models.GetTagsDataRequest, useProcessValueString bool) {

	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName
	tagName := request.TagName

	var data datamodel.DataResponseAny

	if request.TagGroupName != models.CustomTagGroup && request.TagGroupName != models.CustomStringTagGroup {
		helpers.HandleInvalidInputError(c, errors.New("invalid tag group"))
		return
	}

	workCellId, err := GetWorkCellId(enterpriseName, siteName, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	var getCustomTagDataRequest models.GetCustomTagDataRequest

	err = c.BindQuery(&getCustomTagDataRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	if useProcessValueString {
		getCustomTagDataRequest.TimeBucket = "none" //nolint:goconst
		getCustomTagDataRequest.TagAggregates = "null"
		getCustomTagDataRequest.GapFilling = "none"
	}

	timeBucket := getCustomTagDataRequest.TimeBucket

	from := getCustomTagDataRequest.From
	to := getCustomTagDataRequest.To

	zap.S().Debug("from: ", from)
	zap.S().Debug("to: ", to)

	var sqlStatement string
	if timeBucket == "none" {
		zap.S().Debug("timeBucket: none")
		JSONColumnName := enterpriseName + "-" + siteName + "-" + areaName + "-" + productionLineName + "-" + workCellName + "-" + tagName + "-values"
		data.ColumnNames = []string{"timestamp", JSONColumnName}

		if !useProcessValueString {
			// #nosec G201
			sqlStatement = `
SELECT
	asset_id,
	timestamp,
	value
FROM
	processvaluetable
WHERE
    asset_id = $1 AND
    valuename = $2 AND
    timestamp >= $3 AND
    timestamp <= $4
GROUP BY asset_id, timestamp, value
ORDER BY timestamp
`
		} else {
			sqlStatement = `
SELECT
	asset_id,
	timestamp,
	value
FROM
	processvaluestringtable
WHERE
    asset_id = $1 AND
    valuename = $2 AND
    timestamp >= $3 AND
    timestamp <= $4
GROUP BY asset_id, timestamp, value
ORDER BY timestamp
`
		}
	} else {
		gapFilling := getCustomTagDataRequest.GapFilling

		var gapFillingMethod string

		switch gapFilling {
		case models.NoGapFilling:
			gapFillingMethod = "%s"
		case models.InterpolationGapFilling:
			gapFillingMethod = "interpolate(%s)"
		case models.LocfGapFilling:
			gapFillingMethod = "locf(%s)"
		default:
			helpers.HandleInvalidInputError(
				c,
				fmt.Errorf(
					"invalid gap filling method: %s. Valid values: %s, %s, %s",
					gapFilling,
					models.NoGapFilling,
					models.InterpolationGapFilling,
					models.LocfGapFilling))
			return
		}

		tagAggregates := strings.Split(strings.ReplaceAll(getCustomTagDataRequest.TagAggregates, " ", ""), ",")
		if len(tagAggregates) == 0 {
			helpers.HandleInvalidInputError(c, errors.New("invalid tag aggregates"))
			return
		}

		uniqueTagAggregates := make(map[string]bool)
		for _, tagAggregate := range tagAggregates {
			uniqueTagAggregates[tagAggregate] = true
		}
		tagAggregates = []string{}
		for tagAggregate := range uniqueTagAggregates {
			tagAggregates = append(tagAggregates, tagAggregate)
		}

		sort.Strings(tagAggregates)

		data.ColumnNames = []string{"timestamp"}

		var selectClauseEntries = make([]string, 0)
		for _, tagAggregate := range tagAggregates {
			JSONColumnName := enterpriseName + "-" + siteName + "-" + areaName + "-" + productionLineName + "-" + workCellName + "-" + tagName + "-" + tagAggregate
			data.ColumnNames = append(data.ColumnNames, JSONColumnName)
			var str string
			switch tagAggregate {
			case models.AverageTagAggregate:
				str = fmt.Sprintf("%s(value)", models.AverageTagAggregate)
			case models.CountTagAggregate:
				str = fmt.Sprintf("%s(value)", models.CountTagAggregate)
			case models.MaxTagAggregate:
				str = fmt.Sprintf("%s(value)", models.MaxTagAggregate)
			case models.MinTagAggregate:
				str = fmt.Sprintf("%s(value)", models.MinTagAggregate)
			case models.SumTagAggregate:
				str = fmt.Sprintf("%s(value)", models.SumTagAggregate)
			default:
				helpers.HandleInvalidInputError(
					c,
					fmt.Errorf(
						"invalid tag aggregate: %s. Valid values: %s, %s, %s, %s, %s",
						tagAggregate,
						models.AverageTagAggregate,
						models.CountTagAggregate,
						models.MaxTagAggregate,
						models.MinTagAggregate,
						models.SumTagAggregate))
				return
			}

			strGF := fmt.Sprintf(gapFillingMethod, str)
			selectClauseEntries = append(selectClauseEntries, strGF+" AS "+tagAggregate)
		}

		zap.S().Debugf("select clause entries: %v", selectClauseEntries)

		selectClause := strings.Join(selectClauseEntries, ", ")

		if len(selectClauseEntries) > 0 {
			selectClause = ", " + selectClause
		}

		var bucketToDuration time.Duration
		bucketToDuration, err = timeBucketToDuration(timeBucket)
		if err != nil {
			helpers.HandleInvalidInputError(c, err)
			return
		}

		zap.S().Debug("bucketToDuration: ", bucketToDuration)

		// round from and to to the nearest bucket
		fromX := from.Truncate(bucketToDuration)
		zap.S().Debug("fromX: ", fromX)
		toX := to.Truncate(bucketToDuration)
		zap.S().Debug("toX: ", toX)

		fromMinusBucketSize := fromX.Add(bucketToDuration * -1)
		toPlusBucketSize := toX.Add(bucketToDuration)

		zap.S().Debug("fromMinusBucketSize: ", fromMinusBucketSize)
		zap.S().Debug("toPlusBucketSize: ", toPlusBucketSize)

		if helpers.StrToBool(getCustomTagDataRequest.IncludeNext) {
			zap.S().Debug("Including next")
			to, err = QueryInterpolationPoint(workCellId, tagName, toPlusBucketSize)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}
		}

		if helpers.StrToBool(getCustomTagDataRequest.IncludePrevious) {
			zap.S().Debug("Including previous")
			from, err = QueryLOCFPoint(workCellId, tagName, fromMinusBucketSize)
			if err != nil {
				helpers.HandleInternalServerError(c, err)
				return
			}
		}
		// #nosec G201
		{
			sqlStatement = fmt.Sprintf(
				`
SELECT
    time_bucket_gapfill('%s', timestamp) AS bucket,
    asset_id
    %s
FROM
    processvaluetable
WHERE
        asset_id = $1 AND
        valuename = $2 AND
        timestamp >= $3 AND
        timestamp <= $4
GROUP BY bucket, asset_id
ORDER BY bucket;
`, timeBucket, selectClause)
		}

	}

	if from.After(to) {
		helpers.HandleInvalidInputError(c, errors.New("invalid time range (from > to)"))
		return
	}

	zap.S().Debugf("sqlStatement: %s", sqlStatement)
	zap.S().Debugf("workCellId: %d", workCellId)
	zap.S().Debugf("tagName: %s", tagName)
	zap.S().Debugf("from: %s", from)
	zap.S().Debugf("to: %s", to)
	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement, workCellId, tagName, from, to)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	for rows.Next() {
		row := make([]interface{}, len(cols))
		for i := range row {
			row[i] = new(interface{})
		}
		err = rows.Scan(row...)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
		zap.S().Debugf("row (A): %v", row)

		var rowX = make([]interface{}, len(row)-1)

		// if timeBucket is set, include timestamp in response
		if timeBucket != "none" {

			r0 := row[0]
			// convert *interface{} to string
			timestamp := fmt.Sprintf("%v", *r0.(*interface{}))
			//2023-01-01 01:00:00 +0000 +0000
			timestamp = strings.ReplaceAll(timestamp, " +0000 +0000", "")
			timestamp = strings.ReplaceAll(timestamp, " +0000", "")
			timestamp = strings.ReplaceAll(timestamp, " UTC", "")

			//2023-01-01 01:00:00
			// parse to time.time
			var t time.Time
			t, err = time.Parse("2006-01-02 15:04:05", timestamp)
			if err != nil {
				database.ErrorHandling(sqlStatement, err, false)
				return
			}

			// time.time as rfc 3339
			row[0] = t.Format(time.RFC3339)

		}

		zap.S().Debugf("row (B): %v", row)
		// row without asset_id
		hasValue := false
		n := 0
		for i := range row {
			if i == internal.IndexOf(cols, "asset_id") { //This skips row 0
				continue
			}
			rowX[n] = row[i]
			n++
			if row[i] != nil {
				hasValue = true
			}
		}

		// Prevents null value inclusion
		if hasValue {
			zap.S().Debugf("hasValue: %v", hasValue)
			data.Datapoints = append(data.Datapoints, rowX)
		}
	}
	zap.S().Debugf("dp.Datapoints: %d", len(data.Datapoints))

	c.JSON(http.StatusOK, data)
}

func timeBucketToDuration(timeBucket string) (duration time.Duration, err error) {
	// check if timebucket is valid
	var validTimeBucket = regexp.MustCompile(`^\d+[mhdwMy]$|^none$`)

	if !validTimeBucket.MatchString(timeBucket) {
		err = fmt.Errorf("invalid time bucket: %s", timeBucket)
		return
	}

	var timeBucketUnit = timeBucket[len(timeBucket)-1:]
	var timeBucketSize = timeBucket[:len(timeBucket)-1]

	var timeBucketSizeInt int
	timeBucketSizeInt, err = strconv.Atoi(timeBucketSize)
	if err != nil {
		return 0, errors.New("invalid time bucket")
	}

	switch timeBucketUnit {
	case "y":
		duration = time.Duration(timeBucketSizeInt) * (time.Hour * 24 * 365)
	case "M":
		// 31 is safe here !
		duration = time.Duration(timeBucketSizeInt) * (time.Hour * 24 * 31)
	case "w":
		duration = time.Duration(timeBucketSizeInt) * (time.Hour * 24 * 7)
	case "d":
		duration = time.Duration(timeBucketSizeInt) * (time.Hour * 24)
	case "h":
		duration = time.Duration(timeBucketSizeInt) * time.Hour
	case "m":
		duration = time.Duration(timeBucketSizeInt) * time.Minute
	default:
		return 0, errors.New("invalid time bucket")
	}
	return duration, nil
}

func QueryLOCFPoint(workCellId uint32, tagName string, from time.Time) (time.Time, error) {
	sqlStatement := `
SELECT
    timestamp
FROM
    processvaluetable
WHERE
	asset_id=$1 AND
	valuename = $2 AND
	timestamp < $3
ORDER BY 
	timestamp DESC
LIMIT 1
`
	var timestamp time.Time
	err := database.Db.QueryRow(sqlStatement, workCellId, tagName, from).Scan(&timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		timestamp = from
		return timestamp, nil
	}
	return timestamp, err
}

func QueryInterpolationPoint(workCellId uint32, tagName string, to time.Time) (time.Time, error) {
	sqlStatement := `
SELECT
                        timestamp
                    FROM processvaluetable
                    WHERE
                            asset_id = $1 AND
                            valuename = $2 AND
                            timestamp >= $3
                    ORDER BY timestamp
                    LIMIT 1
`
	var timestamp time.Time
	err := database.Db.QueryRow(sqlStatement, workCellId, tagName, to).Scan(&timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		timestamp = to
		return timestamp, nil
	}
	return timestamp, err
}

type TagsWithExpiry struct {
	expiry time.Time
	tag    []string
}

var prefetchedTags = make(map[uint32]TagsWithExpiry)
var prefetchedStringTags = make(map[uint32]TagsWithExpiry)

func GetPrefetchedTags(workCellId uint32, isPVS bool) ([]string, bool) {
	if isPVS {
		// Check if tags are in map
		if tags, ok := prefetchedTags[workCellId]; ok {
			// Check if tags are still valid
			if time.Now().Before(tags.expiry) {
				return tags.tag, true
			}
		}
	} else {
		// Check if tags are in map
		if tags, ok := prefetchedStringTags[workCellId]; ok {
			// Check if tags are still valid
			if time.Now().Before(tags.expiry) {
				return tags.tag, true
			}
		}
	}
	return nil, false
}

func TagPrefetch() {
	// Prefetch all at startup
	workCellIds, err := GetAllWorkCellIds()
	if err != nil {
		zap.S().Errorf("Error while prefetching tags: %v", err)
		return
	}

	var waitGroupPrefetch sync.WaitGroup
	waitGroupPrefetch.Add(len(workCellIds))

	// Split for into 16 goroutines

	for i := 0; i < len(workCellIds); i += 16 {
		end := i + 16
		if end > len(workCellIds) {
			end = len(workCellIds)
		}
		go func(workCellIds []uint32) {
			for _, workCellId := range workCellIds {
				err = prefetch(workCellId)
				if err != nil {
					zap.S().Errorf("Error while prefetching tag for workCellId %d: %v", workCellIds, err)
				}
			}
			waitGroupPrefetch.Done()
		}(workCellIds[i:end])
	}

	waitGroupPrefetch.Wait()

	zap.S().Infof("Prefetching tags finished")

	for {
		// Sleep for 10 seconds
		time.Sleep(10 * time.Second)
		workCellIds, err = GetAllWorkCellIds()
		if err != nil {
			zap.S().Errorf("Error while prefetching tags: %v", err)
			continue
		}

		// Select all workCellIds that are not in the map
		for _, workCellId := range workCellIds {
			if _, ok := prefetchedTags[workCellId]; !ok {
				zap.S().Debugf("Prefetching new workCellId %d", workCellId)
				err = prefetch(workCellId)
				if err != nil {
					zap.S().Errorf("Error while prefetching tag for workCellId %d: %v", workCellIds, err)
				}
			}
		}

		// Select all expired workCellIds
		for workCellId, tags := range prefetchedTags {
			if time.Now().After(tags.expiry) {
				zap.S().Debugf("Prefetching expired workCellId %d", workCellId)
				err = prefetch(workCellId)
				if err != nil {
					zap.S().Errorf("Error while prefetching tag for workCellId %d: %v", workCellIds, err)
				}
			}
		}
	}
}

var inProgressUpdate = make(map[uint32]bool)
var inProgressUpdateMutex = &sync.Mutex{}

func prefetch(workCellId uint32) error {
	// Check if workCellId is already in progress
	inProgressUpdateMutex.Lock()
	if _, ok := inProgressUpdate[workCellId]; ok {
		zap.S().Debugf("Prefetching tags for workCellId %d already in progress", workCellId)
		inProgressUpdateMutex.Unlock()
		return nil
	}
	inProgressUpdate[workCellId] = true
	inProgressUpdateMutex.Unlock()

	zap.S().Debugf("Prefetching tags for workCellId %d", workCellId)
	tags, err := getCustomTags(workCellId, false)
	if err != nil {
		return err
	}

	updateWorkCellTag(workCellId, tags, false)

	stringTags, err := getCustomTags(workCellId, true)
	if err != nil {
		return err
	}
	updateWorkCellTag(workCellId, stringTags, true)

	inProgressUpdateMutex.Lock()
	delete(inProgressUpdate, workCellId)
	inProgressUpdateMutex.Unlock()
	return nil
}

func updateWorkCellTag(workCellId uint32, tag []string, isPVS bool) {

	rnd, err := rand.Int(rand.Reader, big.NewInt(300))
	if err != nil {
		zap.S().Errorf("Error while generating random number: %v", err)
		return
	}
	tenMinutesPlusRandom := time.Now().Add(10 * time.Minute).Add(time.Duration(rnd.Int64()) * time.Second)

	if isPVS {
		prefetchedTags[workCellId] = TagsWithExpiry{
			tag:    tag,
			expiry: tenMinutesPlusRandom,
		}
	} else {
		prefetchedStringTags[workCellId] = TagsWithExpiry{
			tag:    tag,
			expiry: tenMinutesPlusRandom,
		}
	}
}

func getCustomTags(workCellId uint32, isPVS bool) (tags []string, err error) {
	var sqlStatement string
	if isPVS {
		sqlStatement = `SELECT DISTINCT valueName FROM processValueStringTable WHERE asset_id = $1`
	} else {
		sqlStatement = `SELECT DISTINCT valueName FROM processValueTable WHERE asset_id = $1`
	}
	rows, err := database.Db.Query(sqlStatement, workCellId)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	defer rows.Close()

	for rows.Next() {
		var valueName string
		err = rows.Scan(&valueName)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}

		tags = append(tags, valueName)
	}

	return
}
