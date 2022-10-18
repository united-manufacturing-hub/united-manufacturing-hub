package services

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/repository"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
	customTagGroupExists, err = GetCustomTagsExists(id)
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

func GetCustomTags(workCellId uint32) (grouping map[string][]string, err error) {
	grouping = make(map[string][]string)
	zap.S().Infof(
		"[GetTags] Getting custom tags for work cell %s", workCellId)

	sqlStatement := `SELECT DISTINCT valueName FROM processValueTable WHERE asset_id = $1`

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

		if strings.Count(valueName, "_") == 1 {
			if !strings.HasPrefix(valueName, "_") && !strings.HasSuffix(valueName, "_") {
				left, right, _ := strings.Cut(valueName, "_")
				grouping[left] = append(grouping[left], right)
			}
		} else if strings.Count(valueName, "_") == 0 {
			grouping[valueName] = []string{}
		}
	}

	return
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
	keepStatesInteger := getStateTagRequest.KeepStatesInteger

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

	// raw states from database
	rawStates, err := GetStatesRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get shifts for noShift detection
	rawShifts, err := GetShiftsRaw(workCellId, from, to, configuration)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get counts for lowSpeed detection
	countSlice, err := GetCountsRaw(workCellId, from, to)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	// get orders for changeover detection
	orderArray, err := GetOrdersRaw(workCellId, from, to)
	if err != nil {
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

func ProcessCustomTagRequest(c *gin.Context, request models.GetTagsDataRequest) {

	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName
	tagName := request.TagName

	var data datamodel.DataResponseAny

	if request.TagGroupName != models.CustomTagGroup {
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

	gapFilling := getCustomTagDataRequest.GapFilling
	timeBucket := getCustomTagDataRequest.TimeBucket
	// check if timebucket is valid
	splitTimeBucket := strings.Split(timeBucket, " ")
	if len(splitTimeBucket) != 2 {
		helpers.HandleInvalidInputError(c, errors.New("invalid timebucket, requires number and interval"))
		return
	}
	_, err = strconv.Atoi(splitTimeBucket[0])
	if err != nil {
		helpers.HandleInvalidInputError(c, errors.New("invalid timebucket, first part is not a number"))
		return
	}
	switch strings.ToLower(splitTimeBucket[1]) {
	case "year":
	case "month":
	case "week":
	case "day":
	case "hour":
	case "minute":
	case "second":
	default:
		helpers.HandleInvalidInputError(c, errors.New("invalid timebucket, second part is not a valid interval"))
	}

	from := getCustomTagDataRequest.From
	to := getCustomTagDataRequest.To

	if getCustomTagDataRequest.IncludeNext != nil && *getCustomTagDataRequest.IncludeNext {
		to, err = QueryInterpolationPoint(workCellId, tagName, to)
		if err != nil {
			helpers.HandleInternalServerError(c, err)
			return
		}
	}

	if getCustomTagDataRequest.IncludePrevious != nil && *getCustomTagDataRequest.IncludePrevious {
		from, err = QueryLOCFPoint(workCellId, tagName, from)
		if err != nil {
			helpers.HandleInternalServerError(c, err)
			return
		}
	}

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

	var sqlStatement string
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

	if from.After(to) {
		helpers.HandleInvalidInputError(c, errors.New("invalid time range (from > to)"))
		return
	}

	zap.S().Debugf("sqlStatement: %s", sqlStatement)
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

		r0 := row[0]
		// convert *interface{} to string
		timestamp := fmt.Sprintf("%v", *r0.(*interface{}))
		//2023-01-01 01:00:00 +0000 +0000
		timestamp = strings.ReplaceAll(timestamp, " +0000 +0000", "")

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

		// row without row 1, but including 0

		rowX := make([]interface{}, len(row)-1)
		n := 0
		for i := range row {
			if i == 1 {
				continue
			}
			rowX[n] = row[i]
			n++
		}

		data.Datapoints = append(data.Datapoints, rowX)
	}
	c.JSON(http.StatusOK, data)
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
