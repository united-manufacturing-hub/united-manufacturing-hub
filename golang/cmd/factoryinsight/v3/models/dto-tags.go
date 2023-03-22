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

/*


import "time"

type TagGroup struct {
	Name string `json:"name"`
	Id   int    `json:"id"`
}

type GetTagGroupsResponse struct {
	TagGroups []TagGroup `json:"tagGroups"`
}

type GetTagGroupsRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
	WorkCellName       string `uri:"workCellName" binding:"required"`
}

type Tag struct {
	Name string `json:"name"`
	Id   int    `json:"id"`
}

type GetTagsResponse struct {
	Tags []Tag `json:"tags"`
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
*/
