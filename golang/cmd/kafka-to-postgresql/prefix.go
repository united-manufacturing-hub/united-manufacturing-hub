package main

var Prefix = newPrefixRegistry()

func newPrefixRegistry() *prefixRegistry {

	return &prefixRegistry{
		// Undocumented
		AddMaintenanceActivity: "addMaintenanceActivity",
		AddOrder:               "addOrder",
		AddParentToChild:       "addParentToChild",
		AddProduct:             "addProduct",
		AddShift:               "addShift",
		Count:                  "count",
		// Undocumented
		DeleteShiftByAssetIdAndBeginTimestamp: "deleteShiftByAssetIdAndBeginTimestamp",
		// Undocumented
		DeleteShiftById: "deleteShiftById",
		EndOrder:        "endOrder",
		// Undocumented
		ModifyProducesPieces: "modifyProducedPieces",
		// Undocumented
		ModifyState:  "modifyState",
		ProcessValue: "processValue",
		// Digital shadow
		ProcessValueFloat64: "processValueFloat64",
		// Digital shadow
		ProcessValueString: "processValueString",
		ProductTag:         "productTag",
		ProductTagString:   "productTagString",
		// Undocumented
		Recommendation:     "recommendation",
		ScrapCount:         "scrapCount",
		StartOrder:         "startOrder",
		State:              "state",
		UniqueProduct:      "uniqueProduct",
		ScrapUniqueProduct: "scrapUniqueProduct",
	}
}

type prefixRegistry struct {
	AddMaintenanceActivity                string
	AddOrder                              string
	AddParentToChild                      string
	AddProduct                            string
	AddShift                              string
	Count                                 string
	DeleteShiftByAssetIdAndBeginTimestamp string
	DeleteShiftById                       string
	EndOrder                              string
	ModifyProducesPieces                  string
	ModifyState                           string
	ProcessValue                          string
	ProcessValueFloat64                   string
	ProcessValueString                    string
	ProductTag                            string
	ProductTagString                      string
	Recommendation                        string
	ScrapCount                            string
	StartOrder                            string
	State                                 string
	UniqueProduct                         string
	ScrapUniqueProduct                    string
}
