package models

type GetProductionLineRequest struct {
	EnterpriseName string `uri:"enterpriseName" binding:"required"`
	SiteName       string `uri:"siteName" binding:"required"`
	AreaName       string `uri:"areaName" binding:"required"`
}

const (
	MockProductionLine1 = "ProductionLine1"
	MockProductionLine2 = "ProductionLine2"
	MockProductionLine3 = "ProductionLine3"
)
