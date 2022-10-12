package models

type Site struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}
type GetSitesResponse struct {
	Sites []Site `json:"sites"`
}

type GetSiteRequest struct {
	EnterpriseName string `uri:"enterpriseName" binding:"required"`
}
