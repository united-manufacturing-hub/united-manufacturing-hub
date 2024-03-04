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
	Name         string
	IsNumeric    bool
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
		CycleTimeMs       uint64 `json:"cycleTimeMs,omitempty"` //Note: omitempty is not checked when unmarshalling from JSON, and only used as a note for the reader
	} `json:"product"`
	Quantity        uint64 `json:"quantity"`
	Status          Status `json:"status"`
	StartTimeUnixMs uint64 `json:"startTimeUnixMs,omitempty"`
	EndTimeUnixMs   uint64 `json:"endTimeUnixMs,omitempty"`
}

type WorkOrderStartMessage struct {
	ExternalWorkOrderId string `json:"externalWorkOrderId"`
	StartTimeUnixMs     uint64 `json:"startTimeUnixMs"`
}

type WorkOrderStopMessage struct {
	ExternalWorkOrderId string `json:"externalWorkOrderId"`
	EndTimeUnixMs       uint64 `json:"endTimeUnixMs"`
}
