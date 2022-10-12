package models

type ProductionLine struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetProductionLinesResponse struct {
	ProductionLines []ProductionLine `json:"productionLines"`
}

type GetProductionLineRequest struct {
	EnterpriseName string `uri:"enterpriseName" binding:"required"`
	SiteName       string `uri:"siteName" binding:"required"`
	AreaName       string `uri:"areaName" binding:"required"`
}
