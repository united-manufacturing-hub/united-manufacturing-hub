package models

import "time"

type TableType struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
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

const (
	JobTable          string = "job"
	ProductsTable     string = "products"
	ProductTypesTable string = "productTypes"
	// TODO: shopfloor losses tables
)
