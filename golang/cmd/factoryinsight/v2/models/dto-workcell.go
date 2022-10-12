package models

type WorkCell struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetWorkCellsResponse struct {
	WorkCells []WorkCell `json:"workCells"`
}

type GetWorkCellsRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
}
