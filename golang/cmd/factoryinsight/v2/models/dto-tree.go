package models

type TreeStructureEnterpriseMap map[string]TreeStructureEnterprise

type TreeStructureEnterprise struct {
	Sites map[string]TreeStructureSite `json:"sites"`
}
type TreeStructureSite struct {
	Areas map[string]TreeStructureArea `json:"areas"`
}
type TreeStructureArea struct {
	ProductionLines map[string]TreeStructureProductionLines `json:"productionLines"`
}
type TreeStructureProductionLines struct {
	WorkCells map[string]TreeStructureWorkCell `json:"workCells"`
}

type TreeStructureWorkCell struct {
	Tables map[string]TreeStructureTables `json:"tables"`
	KPIs   []string                       `json:"kpi"`
	Tags   TreeStructureTags              `json:"tags"`
}

type TreeStructureTables struct {
	Id int `json:"id"`
}

type TreeStructureTags struct {
	Standard []string            `json:"standard"`
	Custom   map[string][]string `json:"custom"`
}
