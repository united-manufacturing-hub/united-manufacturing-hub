package services

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v3/repository"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"net/http"
	"strings"
	"time"
)

func GetTagGroups(enterpriseName, siteName, areaName, productionLineName, workCellName string) (tagGroups []string, err error) {
	zap.S().Infof(
		"[GetTagGroups] Getting tag groups for enterprise %s, site %s, area %s, production line %s and work cell %s",
		enterpriseName,
		siteName,
		areaName,
		productionLineName,
		workCellName,
	)

	tagGroups = []string{models.StandardTagGroup, models.CustomTagGroup}

	return tagGroups, nil
}

func GetStandardTags() (tags []string, err error) {
	zap.S().Infof("[GetTags] Getting standard tags")

	tags = []string{
		models.JobsStandardTag,
		models.OutputStandardTag,
		models.ShiftsStandardTag,
		models.StateStandardTag,
		models.ThroughputStandardTag,
	}

	return
}

func GetCustomTags(workCellName string) (grouping map[string][]string, err error) {
	zap.S().Infof(
		"[GetTags] Getting custom tags for work cell %s", workCellName)

	sqlStatement := `SELECT DISTINCT valueName FROM processValueTable WHERE asset_id = $1`

	rows, err := db.Query(sqlStatement, workCellName)
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
			if !strings.HasPrefix(valueName, "_") || !strings.HasSuffix(valueName, "_") {
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
	data, err := GetOrdersTimeline(enterpriseName, siteName, areaName, productionLineName, workCellName, workCellId, getJobTagRequest.From, getJobTagRequest.To)
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
	counts, err := GetCounts(enterpriseName, siteName, areaName, productionLineName, workCellName, workCellId, getCountTagRequest.From, getCountTagRequest.To)
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
	shifts, err := GetShifts(enterpriseName, siteName, areaName, productionLineName, workCellName, workCellId, getShiftsTagRequest.From, getShiftsTagRequest.To)
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
	counts, err := GetProductionSpeed(enterpriseName, siteName, areaName, productionLineName, workCellName, workCellId, from, to, aggregationInterval)
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

	tagAggregates := getCustomTagDataRequest.TagAggregates
	gapFilling := getCustomTagDataRequest.GapFilling
	timeBucket := getCustomTagDataRequest.TimeBucket
	from := getCustomTagDataRequest.From
	to := getCustomTagDataRequest.To

	JSONColumnName := enterpriseName + "-" + siteName + "-" + areaName + "-" + productionLineName + "-" + workCellName + "-" + tagName
	data.ColumnNames = []string{"timestamp", JSONColumnName}

	var selectAggregationMethod string
	var gapFillingMethod string

	switch gapFilling {
	case models.NoGapFilling:
		gapFillingMethod = "%s, "
	case models.InterpolationGapFilling:
		gapFillingMethod = "interpolate(%s), "
	case models.LocfGapFilling:
		gapFillingMethod = "locf(%s), "
	}

	for _, tagAggregate := range tagAggregates {
		switch tagAggregate {
		case models.AverageTagAggregate:
			selectAggregationMethod += fmt.Sprintf(gapFillingMethod, models.AverageTagAggregate)
		case models.CountTagAggregate:
			selectAggregationMethod += fmt.Sprintf(gapFillingMethod, models.CountTagAggregate)
		case models.MaxTagAggregate:
			selectAggregationMethod += fmt.Sprintf(gapFillingMethod, models.MaxTagAggregate)
		case models.MinTagAggregate:
			selectAggregationMethod += fmt.Sprintf(gapFillingMethod, models.MinTagAggregate)
		case models.SumTagAggregate:
			selectAggregationMethod += fmt.Sprintf(gapFillingMethod, models.SumTagAggregate)
		}
	}

	selectAggregationMethod = selectAggregationMethod[:len(selectAggregationMethod)-2]

	var aggregatedView string
	var bucketName string

	switch timeBucket {
	case models.MinuteAggregateView:
		aggregatedView = "aggregationTable_minute"
		bucketName = "minute"
	case models.HourAggregateView:
		aggregatedView = "aggregationTable_hour"
		bucketName = "hour"
	case models.DayAggregateView:
		aggregatedView = "aggregationTable_day"
		bucketName = "day"
	case models.WeekAggregateView:
		aggregatedView = "aggregationTable_week"
		bucketName = "week"
	case models.MonthAggregateView:
		aggregatedView = "aggregationTable_month"
		bucketName = "month"
	case models.YearAggregateView:
		aggregatedView = "aggregationTable_year"
		bucketName = "year"
	}

	sqlStatement := fmt.Sprintf(`SELECT
						bucket as %s,
 						%s
					 FROM %s
					 WHERE
					    asset_id = $1 AND
					    valueName = $2 AND
					    bucket BETWEEN $3 AND $4
					 GROUP BY bucket, asset_id, valueName
					 ORDER BY bucket ASC`, bucketName, selectAggregationMethod, aggregatedView)

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, workCellId, tagName, from, to)
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
	colLen := len(cols)
	values := make([]interface{}, colLen)

	for rows.Next() {
		values[0] = new(time.Time)
		for i := 1; i < colLen; i++ {
			values[i] = new(float64)
		}

		err = rows.Scan(values...)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}

		valuesSanitized := []interface{}{
			float64(values[0].(*time.Time).UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))),
			values}
		data.Datapoints = append(data.Datapoints, valuesSanitized)
	}
	c.JSON(http.StatusOK, data)
}
