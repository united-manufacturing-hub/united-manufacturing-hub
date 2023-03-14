// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	Entries map[string]*TreeEntryFormat `json:"entries"`
	Label   string                      `json:"label"`
	Value   string                      `json:"value"`
}

type TreeEntryValues struct {
	Label  string            `json:"label"`
	Value  string            `json:"value"`
	Tables []TreeEntryFormat `json:"tables"`
	KPIs   []TreeEntryFormat `json:"kpi"`
	Tags   []TreeEntryFormat `json:"tags"`
}
