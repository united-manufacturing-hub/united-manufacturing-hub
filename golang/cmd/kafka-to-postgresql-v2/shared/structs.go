package shared

const (
	DbTagSeparator = "$"
)

type TopicDetails struct {
	Enterprise     string
	Site           string
	Area           string
	ProductionLine string
	WorkCell       string
	OriginId       string
	Schema         string
	Tag            string
}

type HistorianValue struct {
	NumericValue *float32
	StringValue  *string
	IsNumeric    bool
	Name         string
}

type Status int

const (
	Planned Status = iota
	InProgress
	Completed
)

type WorkOrderCreateMessage struct {
	ExternalWorkOrderId string `json:"externalWorkOrderId"`
	Product             struct {
		ExternalProductId string `json:"externalProductId"`
		CycleTimeMs       int    `json:"cycleTimeMs"`
	} `json:"product"`
	Quantity        uint64 `json:"quantity"`
	Status          Status `json:"status"`
	StartTimeUnixMs int64  `json:"startTimeUnixMs"`
	EndTimeUnixMs   int64  `json:"endTimeUnixMs"`
}

type WorkOrderStartMessage struct {
	ExternalWorkOrderId string `json:"externalWorkOrderId"`
	StartTimeUnixMs     int64  `json:"startTimeUnixMs"`
}

type WorkOrderStopMessage struct {
	ExternalWorkOrderId string `json:"externalWorkOrderId"`
	EndTimeUnixMs       int64  `json:"endTimeUnixMs"`
}
