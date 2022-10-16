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
	TagAggregates []string `form:"tagAggregates" binding:"required"`
	GapFilling    string   `form:"gapFilling" binding:"required"`
	TimeBucket    string   `form:"timeBucket" binding:"required"`
}

type GetJobTagRequest struct {
	From time.Time `json:"from" binding:"required"`
	To   time.Time `json:"to" binding:"required"`
}

type GetCountTagRequest struct {
	From                time.Time `json:"from" binding:"required"`
	To                  time.Time `json:"to" binding:"required"`
	AccumulatedProducts bool      `json:"accumulatedProducts"`
}

type GetShiftsTagRequest struct {
	From time.Time `json:"from" binding:"required"`
	To   time.Time `json:"to" binding:"required"`
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
	AverageTagAggregate string = "Average"
	CountTagAggregate   string = "Count"
	MaxTagAggregate     string = "Maximum"
	MinTagAggregate     string = "Minimum"
	SumTagAggregate     string = "Sum"
)
