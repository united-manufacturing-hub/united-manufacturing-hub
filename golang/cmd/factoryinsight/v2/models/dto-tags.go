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

package models

import "time"

type GetTagsResponse struct {
	Tags []string
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
	IncludePrevious string `form:"includePrevious" binding:"required"`
	IncludeNext     string `form:"includeNext" binding:"required"`

	GapFilling    string    `form:"gapFilling" binding:"required"`
	TimeBucket    string    `form:"timeBucket" binding:"required"`
	From          time.Time `form:"from" binding:"required"`
	To            time.Time `form:"to" binding:"required"`
	TagAggregates string    `form:"tagAggregates" binding:"required"`
}

type GetJobTagRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type GetCountTagRequest struct {
	From                time.Time `form:"from" binding:"required"`
	To                  time.Time `form:"to" binding:"required"`
	AccumulatedProducts string    `form:"accumulatedProducts"`
}

type GetShiftsTagRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type GetStateTagRequest struct {
	From              time.Time `form:"from" binding:"required"`
	To                time.Time `form:"to" binding:"required"`
	KeepStatesInteger string    `form:"keepStatesInteger"`
}

type GetThroughputTagRequest struct {
	From                time.Time `form:"from" binding:"required"`
	To                  time.Time `form:"to" binding:"required"`
	AggregationInterval int       `form:"aggregationInterval"`
}

const (
	CustomTagGroup       string = "custom"
	StandardTagGroup     string = "standard"
	CustomStringTagGroup string = "customString"
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
