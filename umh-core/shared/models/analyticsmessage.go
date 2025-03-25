package models

type WorkOrderStatus int

const (
	Planned WorkOrderStatus = iota
	InProgress
	Completed
)

type WorkOrderCreateMessageProduct struct {
	ExternalProductId string  `json:"external_product_id"`
	CycleTimeMs       *uint64 `json:"cycle_time_ms,omitempty"`
}

type WorkOrderCreateMessage struct {
	ExternalWorkOrderId string                        `json:"external_work_order_id"`
	Product             WorkOrderCreateMessageProduct `json:"product"`
	Quantity            uint64                        `json:"quantity"`
	Status              WorkOrderStatus               `json:"status"`
	StartTimeUnixMs     *uint64                       `json:"start_time_unix_ms,omitempty"`
	EndTimeUnixMs       *uint64                       `json:"end_time_unix_ms,omitempty"`
}

type WorkOrderStartMessage struct {
	ExternalWorkOrderId string `json:"external_work_order_id"`
	StartTimeUnixMs     uint64 `json:"start_time_unix_ms"`
}

type WorkOrderStopMessage struct {
	ExternalWorkOrderId string `json:"external_work_order_id"`
	EndTimeUnixMs       uint64 `json:"end_time_unix_ms"`
}

type ProductAddMessage struct {
	ExternalProductTypeId string  `json:"external_product_type_id"`
	ProductBatchId        *string `json:"product_batch_id,omitempty"`
	StartTimeUnixMs       *uint64 `json:"start_time_unix_ms,omitempty"`
	EndTimeUnixMs         uint64  `json:"end_time_unix_ms"`
	Quantity              uint64  `json:"quantity"`
	BadQuantity           *uint64 `json:"bad_quantity,omitempty"`
}

type ProductSetBadQuantityMessage struct {
	ExternalProductId string `json:"external_product_id"`
	EndTimeUnixMs     uint64 `json:"end_time_unix_ms"`
	BadQuantity       uint64 `json:"bad_quantity"`
}

type ProductTypeCreateMessage struct {
	ExternalProductTypeId string `json:"external_product_type_id"`
	CycleTimeMs           uint64 `json:"cycle_time_ms"`
}

type ShiftAddMessage struct {
	StartTimeUnixMs uint64 `json:"start_time_unix_ms"`
	EndTimeUnixMs   uint64 `json:"end_time_unix_ms"`
}

type ShiftDeleteMessage struct {
	StartTimeUnixMs uint64 `json:"start_time_unix_ms"`
}

type StateAddMessage struct {
	StartTimeUnixMs uint64 `json:"start_time_unix_ms"`
	State           uint64 `json:"state"`
}

type StateOverwriteMessage struct {
	StartTimeUnixMs uint64 `json:"start_time_unix_ms"`
	EndTimeUnixMs   uint64 `json:"end_time_unix_ms"`
	State           uint64 `json:"state"`
}
