package models

type GetAreasRequest struct {
	EnterpriseName string `uri:"enterpriseName" binding:"required"`
	SiteName       string `uri:"siteName" binding:"required"`
}

const (
	MockArea1 = "Area1"
	MockArea2 = "Area2"
	MockArea3 = "Area3"
)
