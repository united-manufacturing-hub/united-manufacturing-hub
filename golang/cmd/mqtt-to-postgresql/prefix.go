package main

var Prefix = newPrefixRegistry()

func newPrefixRegistry() *prefixRegistry {

	return &prefixRegistry{
		ProcessValueFloat64:                   "processValueFloat64",
		ProcessValue:                          "processValue",
		ProcessValueString:                    "processValueString",
		Count:                                 "count",
		Recommendation:                        "recommendation",
		State:                                 "state",
		UniqueProduct:                         "uniqueProduct",
		ScrapCount:                            "scrapCount",
		AddShift:                              "addShift",
		UniqueProductScrap:                    "uniqueProductScrap",
		AddProduct:                            "addProduct",
		AddOrder:                              "addOrder",
		StartOrder:                            "startOrder",
		EndOrder:                              "endOrder",
		AddMaintenanceActivity:                "addMaintenanceActivity",
		ProductTag:                            "productTag",
		ProductTagString:                      "productTagString",
		AddParentToChild:                      "addParentToChild",
		ModifyState:                           "modifyState",
		ModifyProducedPieces:                  "modifyProducedPieces",
		DeleteShiftById:                       "deleteShiftById",
		DeleteShiftByAssetIdAndBeginTimestamp: "deleteShiftByAssetIdAndBeginTimestamp",
		//For internal use only !
		RawMQTTRequeue: "rawMQTTRequeue",
	}
}

type prefixRegistry struct {
	ProcessValueFloat64                   string
	ProcessValue                          string
	ProcessValueString                    string
	Count                                 string
	Recommendation                        string
	State                                 string
	UniqueProduct                         string
	ScrapCount                            string
	AddShift                              string
	UniqueProductScrap                    string
	AddProduct                            string
	AddOrder                              string
	StartOrder                            string
	EndOrder                              string
	AddMaintenanceActivity                string
	ProductTag                            string
	ProductTagString                      string
	AddParentToChild                      string
	ModifyState                           string
	ModifyProducedPieces                  string
	DeleteShiftById                       string
	DeleteShiftByAssetIdAndBeginTimestamp string
	RawMQTTRequeue                        string
}
