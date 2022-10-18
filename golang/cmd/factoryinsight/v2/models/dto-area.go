package models

type GetAreasRequest struct {
	EnterpriseName string `uri:"enterpriseName" binding:"required"`
	SiteName       string `uri:"siteName" binding:"required"`
}

const (
	MockDefaultArea = "DefaultArea"
)
