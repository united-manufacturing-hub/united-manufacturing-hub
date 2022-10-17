package models

import "time"

type TableType struct {
	Name string `json:"name"`
	Id   int    `json:"id"`
}

type GetTableTypesResponse struct {
	Tables []TableType `json:"tables"`
}

type GetTableTypesRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
	WorkCellName       string `uri:"workCellName" binding:"required"`
}

type GetTableDataRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
	WorkCellName       string `uri:"workCellName" binding:"required"`
	TableType          string `uri:"tableType" binding:"required"`
}

type GetJobTableRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type GetProductsTableRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type GetProductTypesTableRequest struct {
	From time.Time `form:"from" binding:"required"`
	To   time.Time `form:"to" binding:"required"`
}

type GetAvailabilityHistogramRequest struct {
	From              time.Time `form:"from" binding:"required"`
	To                time.Time `form:"to" binding:"required"`
	IncludeRunning    *bool     `form:"includeRunning"`    // *bool is required, see also https://github.com/gin-gonic/gin/issues/814
	KeepStatesInteger *bool     `form:"keepStatesInteger"` // *bool is required, see also https://github.com/gin-gonic/gin/issues/814
}

type GetAggregatedStatesRequest struct {
	From              time.Time `form:"from" binding:"required"`
	To                time.Time `form:"to" binding:"required"`
	IncludeRunning    *bool     `form:"includeRunning" binding:"required"` // *bool is required, see also https://github.com/gin-gonic/gin/issues/814
	KeepStatesInteger *bool     `form:"keepStatesInteger"`                 // *bool is required, see also https://github.com/gin-gonic/gin/issues/814
	AggregationType   int       `form:"aggregationType"`
}

const (
	JobsTable                  string = "jobs"
	ProductsTable              string = "products"
	ProductTypesTable          string = "productTypes"
	AvailabilityHistogramTable string = "availabilityHistogram"
	AvailabilityTotalTable     string = "availabilityTotal"
	PerformanceTable           string = "performance"
	QualityTable               string = "quality"
)
