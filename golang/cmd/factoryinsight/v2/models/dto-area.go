package models

type Area struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetAreasResponse struct {
	Areas []Area `json:"areas"`
}

type GetAreasRequest struct {
	EnterpriseName string `uri:"enterpriseName" binding:"required"`
	SiteName       string `uri:"siteName" binding:"required"`
}
