package models

import "time"

type GetTagsResponse[T []string | map[string][]string] struct {
	Tags T
}
type GetTagGroupsRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
	WorkCellName       string `uri:"workCellName" binding:"required"`
}

type GetTagsRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
	WorkCellName       string `uri:"workCellName" binding:"required"`
	TagGroupName       string `uri:"tagGroupName" binding:"required"`
}

type GetTagsDataRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
	WorkCellName       string `uri:"workCellName" binding:"required"`
	TagGroupName       string `uri:"tagGroupName" binding:"required"`
	TagName            string `uri:"tagName" binding:"required"`
}

type GetCustomTagDataRequest struct {
	GapFilling    string    `form:"gapFilling" binding:"required"`
	TimeBucket    string    `form:"timeBucket" binding:"required"`
	From          time.Time `form:"from" binding:"required"`
	To            time.Time `form:"to" binding:"required"`
	TagAggregates []string  `form:"tagAggregates" binding:"required"`
}

type GetJobTagRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type GetCountTagRequest struct {
	From                time.Time `form:"from" binding:"required"`
	To                  time.Time `form:"to" binding:"required"`
	AccumulatedProducts bool      `form:"accumulatedProducts"`
}

type GetShiftsTagRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type GetStateTagRequest struct {
	From              time.Time `form:"from" binding:"required"`
	To                time.Time `form:"to" binding:"required"`
	KeepStatesInteger bool      `form:"keepStatesInteger"`
}

type GetThroughputTagRequest struct {
	From                time.Time `form:"from" binding:"required"`
	To                  time.Time `form:"to" binding:"required"`
	AggregationInterval int       `form:"aggregationInterval"`
}

const (
	CustomTagGroup   string = "custom"
	StandardTagGroup string = "standard"
)

const (
	JobsStandardTag       string = "jobs"
	OutputStandardTag     string = "output"
	ShiftsStandardTag     string = "shifts"
	StateStandardTag      string = "state"
	ThroughputStandardTag string = "throughput"
)

const (
	AverageTagAggregate string = "avg"
	CountTagAggregate   string = "count"
	MaxTagAggregate     string = "max"
	MinTagAggregate     string = "min"
	SumTagAggregate     string = "sum"
)

const (
	MinuteAggregateView string = "minute"
	HourAggregateView   string = "hour"
	DayAggregateView    string = "day"
	WeekAggregateView   string = "week"
	MonthAggregateView  string = "month"
	YearAggregateView   string = "year"
)

const (
	NoGapFilling            string = "null"
	InterpolationGapFilling string = "interpolation"
	LocfGapFilling          string = "locf"
)
