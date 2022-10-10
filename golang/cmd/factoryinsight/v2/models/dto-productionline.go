package models

type ProductionLine struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetProductionLinesResponse struct {
	ProductionLines []ProductionLine `json:"productionLines"`
}
