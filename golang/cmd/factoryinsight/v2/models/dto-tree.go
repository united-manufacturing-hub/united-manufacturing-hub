package models

/*
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
	Tags   TreeStructureTags              `json:"tags"`
	Tables map[string]TreeStructureTables `json:"tables"`
	KPIs   []string                       `json:"kpi"`
}

type TreeStructureTables struct {
	Id int `json:"id"`
}

type TreeStructureTags struct {
	Custom   map[string][]string `json:"custom"`
	Standard []string            `json:"standard"`
}*/

type TreeEntryFormat struct {
	Label   string        `json:"label"`
	Value   string        `json:"value"`
	Entries []interface{} `json:"entries"`
}

type TreeEntryValues struct {
	Label  string            `json:"label"`
	Value  string            `json:"value"`
	Tables []TreeEntryFormat `json:"tables"`
	KPIs   []TreeEntryFormat `json:"kpi"`
	Tags   []TreeEntryFormat `json:"tags"`
}
