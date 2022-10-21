package models

type GetSitesRequest struct {
	EnterpriseName string `uri:"enterpriseName" binding:"required"`
}
