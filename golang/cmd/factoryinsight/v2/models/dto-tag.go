package models

type TagGroup struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
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
	Id   int    `json:"id"`
	Name string `json:"name"`
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
