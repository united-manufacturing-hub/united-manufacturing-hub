package models

type Kpi struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetKpisResponse struct {
	Kpis []Kpi `json:"oee"`
}

type GetKpisRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
	WorkCellName       string `uri:"workCellName" binding:"required"`
}

const (
	OeeKpi          string = "oee"
	AvailabilityKpi string = "availability"
	PerformanceKpi  string = "performance"
	QualityKpi      string = "quality"
)
