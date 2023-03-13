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

package main

var Prefix = newPrefixRegistry()

func newPrefixRegistry() *prefixRegistry {

	return &prefixRegistry{
		// Undocumented
		AddMaintenanceActivity: "addmaintenanceactivity",
		AddOrder:               "addorder",
		AddParentToChild:       "addparenttochild",
		AddProduct:             "addproduct",
		AddShift:               "addshift",
		Count:                  "count",
		// Undocumented
		DeleteShift: "deleteshift",
		// Undocumented
		EndOrder: "endorder",
		// Undocumented
		ModifyProducedPieces: "modifyproducedpieces",
		// Undocumented
		ModifyState:  "modifystate",
		ProcessValue: "processvalue",
		// Digital shadow
		ProcessValueFloat64: "processvaluefloat64",
		// Digital shadow
		ProcessValueString: "processvaluestring",
		ProductTag:         "producttag",
		ProductTagString:   "producttagstring",
		// Undocumented
		Recommendation:     "recommendation",
		ScrapCount:         "scrapcount",
		StartOrder:         "startorder",
		State:              "state",
		UniqueProduct:      "uniqueproduct",
		ScrapUniqueProduct: "scrapuniqueproduct",
	}
}

type prefixRegistry struct {
	AddMaintenanceActivity string
	AddOrder               string
	AddParentToChild       string
	AddProduct             string
	AddShift               string
	Count                  string
	DeleteShift            string
	EndOrder               string
	ModifyProducedPieces   string
	ModifyState            string
	ProcessValue           string
	ProcessValueFloat64    string
	ProcessValueString     string
	ProductTag             string
	ProductTagString       string
	Recommendation         string
	ScrapCount             string
	StartOrder             string
	State                  string
	UniqueProduct          string
	ScrapUniqueProduct     string
}
