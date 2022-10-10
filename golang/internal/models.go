package internal

//type StandardTag string
//
//const (
//	Job         StandardTag = "job"
//	Product     StandardTag = "product"
//	ProductType StandardTag = "productType"
//	Shift       StandardTag = "shift"
//	State       StandardTag = "state"
//)

var StandardTags = map[int]string{
	1: "job",
	2: "product",
	3: "productType",
	4: "shift",
	5: "state",
}

var StandardMethods = map[int]string{
	1:  "accumulatedProducts",
	2:  "aggregatedStates",
	3:  "availability",
	4:  "averageChangeoverTime",
	5:  "averageCleaningTime",
	6:  "count",
	7:  "currentState",
	8:  "maintenanceActivities",
	9:  "OEE",
	10: "orderTable",
	11: "orderTimeline",
	12: "performance",
	13: "productionSpeed",
	14: "quality",
	15: "qualityRate",
	16: "shifts",
	17: "state",
	18: "stateHistogram",
	19: "timeRange",
	20: "uniqueProductsWithTags",
	21: "unstartedOrderTable",
	22: "upcomingMaintenanceActivities",
}
