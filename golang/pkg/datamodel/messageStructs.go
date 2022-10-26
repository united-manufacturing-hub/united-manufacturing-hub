package datamodel

/*
 * Based on https://docs.umh.app/docs/concepts/mqtt/ (22.04.2022)
 */

// Deprecated: use ProductAdd instead for new data model
type Count struct {
	Count       uint32 `json:"count"`
	Scrap       uint32 `json:"scrap"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// ProductAdd is new datamodel version of Count
type ProductAdd struct {
	Timestampend   uint64 `json:"timestamp-end"`
	Producttypeid  uint64 `json:"product-type-id"`
	Scrap          uint32 `json:"scrap,omitempty"`
	Id             uint64 `json:"id,omitempty"`
	Timestampbegin uint64 `json:"timestamp-begin,omitempty"`
	Totalamount    uint32 `json:"total-amount,omitempty"`
}

// Deprecated: use StateActivity instead
type Activity struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Activity    bool   `json:"activity"`
}

// StateActivity is new datamodel type version of Activity
type StateActivity struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
	Activity       bool   `json:"activity"`
}

// Deprecated: Use StateReason instead
type DetectedAnomaly struct {
	DetectedAnomaly string `json:"detectedAnomaly"`
	TimestampMs     uint64 `json:"timestamp_ms"`
}

// StateReason is the new datamodel type version of DetectedAnomaly
type StateReason struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
	Reason         string `json:"reason"`
}

// Deprecated: use ShiftAdd instead for the new data model
type AddShift struct {
	TimestampMs    uint64 `json:"timestamp_ms"`
	TimestampMsEnd uint64 `json:"timestamp_ms_end"`
}

// ShiftAdd is new datamodel version of AddShift
type ShiftAdd struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
	Timestampend   uint64 `json:"timestamp-end"`
}

// Deprecated: use ShiftDelete instead for the new datamodel
type DeleteShift struct {
	TimeStampMs uint64 `json:"timestamp_ms"`
}

// ShiftDelete is new datamodel version of DeleteShift
type ShiftDelete struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
}

// Deprecated: use JobAdd instead for the new data model
type AddOrder struct {
	ProductId   string `json:"product_id"`
	OrderId     string `json:"order_id"`
	TargetUnits uint64 `json:"target_units"`
}

// JobAdd is new datamodel version of AddOrder
type JobAdd struct {
	ProductType  string `json:"product-type"`
	Jobid        string `json:"job-id"`
	Targetamount uint64 `json:"target-amount"`
}

// Deprecated: use ProductTypeAdd instead
type AddProduct struct {
	ProductId            string  `json:"product_id"`
	TimePerUnitInSeconds float64 `json:"time_per_unit_in_seconds"`
}

// ProductTypeAdd is new datamodel version of AddProduct
type ProductTypeAdd struct {
	ProductId          string  `json:"product-id"`
	Cycletimeinseconds float64 `json:"cycle-time-in-seconds"`
}

// Deprecated: use JobStart instead with new datamodel
type StartOrder struct {
	OrderId     string `json:"order_id"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// JobStart is new datamodel version of StartOrder
type JobStart struct {
	Jobid          string `json:"job-id"`
	Timestampbegin uint64 `json:"timestamp-begin"`
}

// Deprecated: Use JobEnd instead
type EndOrder struct {
	OrderId     string `json:"order_id"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// JobEnd is new datamodel version of endOrder
type JobEnd struct {
	TimestampEnd uint64 `json:"timestamp-end"`
	Jobid        string `json:"job-id"`
}

// Deprecated: Use ProductOverwrite instead
type ModifyProducedPieces struct {
	Count       int32  `json:"count"`
	Scrap       int32  `json:"scrap"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// ProductOverwrite is new datamodel version of ModifyProducedPieces
type ProductOverwrite struct {
	Timestampend   uint64 `json:"timestamp-end"`
	Id             uint64 `json:"id,omitempty"`
	Timestampbegin uint64 `json:"timestamp-begin,omitempty"`
	Totalamount    int32  `json:"total-amount,omitempty"`
	Scrap          int32  `json:"scrap,omitempty"`
}

// Deprecated: use StateAdd instead for new datamodel
type State struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	State       uint64 `json:"state"`
}

// StateAdd is new datamodel version of State
type StateAdd struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
	State          uint64 `json:"state"`
}

// Deprecated: Use StateOverwrite instead
type ModifyState struct {
	StartTimeStampMs uint64 `json:"timestamp_ms"`
	EndTimeStampMs   uint64 `json:"timestamp_ms_end"`
	NewState         uint32 `json:"new_state"`
}

// StateOverwrite is the new datamodel version of ModifyState
type StateOverwrite struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
	Timestampend   uint64 `json:"timestamp-end"`
	State          uint64 `json:"state"`
}

// Deprecated: Can now be handled by Producttypeadd
type CycleTimeTrigger struct {
	CurrentStation string `json:"currentStation"`
	LastStation    string `json:"lastStation"`
	TimestampMs    uint64 `json:"timestamp_ms"`
	SanityTimeInS  uint64 `json:"sanityTime_in_s"`
}

// Deprecated: part of Digitalshadow messages, which are nonfunctional on 0.10.0
type UniqueProduct struct {
	ProductId                  string `json:"product_id"`
	UniqueProductAlternativeId string `json:"UniqueProductAlternativeId"`
	BeginTimestampMs           uint64 `json:"begin_timestamp_ms"`
	EndTimestampMs             uint64 `json:"end_timestamp_ms"`
	IsScrap                    bool   `json:"is_scrap"`
}

// Deprecated: part of Digitalshadow messages, which are nonfunctional on 0.10.0
type ScrapUniqueProduct struct {
	UID string `json:"uid"`
}

// Deprecated: nonfunctional on 0.10.0
type Recommendations struct {
	RecommendationValues map[string]string `json:"recommendationValues"`
	RecommendationUID    string            `json:"recommendationUID"`
	RecommendationType   string            `json:"recommendationType"`
	TimestampMs          uint64            `json:"timestamp_ms"`
	Enabled              bool              `json:"enabled"`
}

// Deprecated: part of Digitalshadow messages, which are nonfunctional on 0.10.0
type ProductTag struct {
	AID         string `json:"aid"`
	Name        string `json:"name"`
	TimestampMs uint64 `json:"timestamp_ms"`
	Value       int64  `json:"value"`
}

// Deprecated: part of Digitalshadow messages, which are nonfunctional on 0.10.0
type ProductTagString struct {
	AID         string `json:"aid"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// Deprecated: part of Digitalshadow messages, which are nonfunctional on 0.10.0
type AddParentToChild struct {
	ParentAID   string `json:"parentAID"`
	ChildAID    string `json:"childAID"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// Deprecated: can now be handled by Productadd or Productoverwrite
type ScrapCount struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Scrap       uint64 `json:"scrap"`
}

type Barcode struct {
	Barcode     string `json:"barcode"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

type ProductImage struct {
	Image       Image  `json:"image"`
	TimestampMs uint64 `json:"timestamp_ms"`
}
type Image struct {
	ImageId       string `json:"image_id"`
	ImageBytes    string `json:"image_bytes"`
	ImageHeight   uint64 `json:"image_height"`
	ImageWidth    uint64 `json:"image_width"`
	ImageChannels uint8  `json:"image_channels"`
}
