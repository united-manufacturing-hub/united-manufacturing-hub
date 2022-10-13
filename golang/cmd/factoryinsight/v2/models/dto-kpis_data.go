package models

import "time"

type GetKpisDataRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
	WorkCellName       string `uri:"workCellName" binding:"required"`
	KpisMethod         string `uri:"kpisMethod" binding:"required"`
}

type KpiRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}
