package models

type DataFormat struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetDataFormatResponse struct {
	DataFormats []DataFormat `json:"dataFormats"`
}

type GetDataFormatRequest struct {
	EnterpriseName     string `uri:"enterpriseName" binding:"required"`
	SiteName           string `uri:"siteName" binding:"required"`
	AreaName           string `uri:"areaName" binding:"required"`
	ProductionLineName string `uri:"productionLineName" binding:"required"`
	WorkCellName       string `uri:"workCellName" binding:"required"`
}

const (
	TagsDataFormat  string = "tags"
	KpisDataFormat  string = "kips"
	ListsDataFormat string = "lists"
)
