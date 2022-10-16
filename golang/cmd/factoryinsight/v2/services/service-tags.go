package services

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/repository"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"net/http"
	"strings"
	"time"
)

func GetTagGroups(
	enterpriseName string,
	siteName string,
	areaName string,
	productionLineName string,
	workCellName string,
) (tagGroups []string, err error) {
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
	// TODO adapt this to the new data model

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
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
	data, err := GetOrdersTimeline(workCellId, getJobTagRequest.From, getJobTagRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	c.JSON(http.StatusOK, data)
}

func ProcessOutputTagRequest(c *gin.Context, request models.GetTagsDataRequest) {
	// TODO adapt this to the new data model

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
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
	counts, err = GetCounts(workCellId, getCountTagRequest.From, getCountTagRequest.To)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

func ProcessShiftsTagRequest(c *gin.Context, request models.GetTagsDataRequest) {
	// TODO adapt this to the new data model

	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
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
	shifts, err := GetShifts(enterpriseName, workCellId, getShiftsTagRequest.From, getShiftsTagRequest.To)
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

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
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
	processedStates, err := processStatesOptimized(
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
	JSONColumnName := customer + "-" + location + "-" + asset + "-" + "state"
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
	// TODO adapt this to the new data model
	// ### store request in proper variables ###
	enterpriseName := request.EnterpriseName
	siteName := request.SiteName
	areaName := request.AreaName
	productionLineName := request.ProductionLineName
	workCellName := request.WorkCellName

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
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
	counts, err = GetProductionSpeed(workCellId, from, to, aggregationInterval)
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

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
	if err != nil {
		helpers.HandleInternalServerError(c, err)
		return
	}

	var getCustomTagDataRequest models.GetCustomTagDataRequest

	err := c.BindQuery(&getCustomTagDataRequest)
	if err != nil {
		helpers.HandleInvalidInputError(c, err)
		return
	}

	tagAggregates := getCustomTagDataRequest.TagAggregates
	gapFilling := getCustomTagDataRequest.GapFilling
	timeBucket := getCustomTagDataRequest.TimeBucket

	if internal.Contains(tagAggregates, models.AverageTagAggregate) {

		sqlStatement := `SELECT time_bucket(INTERVAL $1, timestamp) AS bucket, AVG(value) FROM processValueTable WHERE asset_id = $2 AND valueName $3`

		var rows *sql.Rows
		rows, err = db.Query(sqlStatement, timeBucket, workCellId, tagName)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}

		defer rows.Close()

		for rows.Next() {
			var timestamp time.Time
			var value float64
			err = rows.Scan(&timestamp, &value)
			if err != nil {
				database.ErrorHandling(sqlStatement, err, false)
				return
			}
		}
	}

	var aggregateStatement string

	for _, tagAggregate := range tagAggregates {
		switch tagAggregate {
		case models.AverageTagAggregate:
			aggregateStatement += "AVG(value), "
		case models.CountTagAggregate:
			aggregateStatement += "COUNT(value), "
		case models.MaxTagAggregate:
			aggregateStatement += "MAX(value), "
		case models.MinTagAggregate:
			aggregateStatement += "MIN(value), "
		case models.SumTagAggregate:
			aggregateStatement += "SUM(value), "
		}
	}

	aggregateStatement = aggregateStatement[:len(aggregateStatement)-2]

	sqlStatement := `SELECT time_bucket(INTERVAL $1, timestamp) AS bucket, $2 FROM processValueTable WHERE asset_id = $3 AND valueName $4`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, timeBucket, aggregateStatement, workCellId, tagName)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	defer rows.Close()

	for rows.Next() {
		var timestamp time.Time
		var value float64
		err = rows.Scan(&timestamp, &value)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
	}
}
