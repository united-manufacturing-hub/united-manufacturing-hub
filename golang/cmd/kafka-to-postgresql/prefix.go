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
		ModifyProducesPieces: "modifyproducedpieces",
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
	ModifyProducesPieces   string
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
