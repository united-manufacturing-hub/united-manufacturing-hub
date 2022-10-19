package datamodel

/*
 * Based on https://docs.umh.app/docs/concepts/mqtt/ (22.04.2022)
 */

// Deprecated: use Productadd instead for new data model
type Count struct {
	Count       uint32 `json:"count"`
	Scrap       uint32 `json:"scrap"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// Productadd is new datamodel version of Count
type Productadd struct {
	Timestampend   uint64 `json:"timestamp-end"`
	Producttypeid  uint64 `json:"product-type-id"`
	Scrap          uint32 `json:"scrap,omitempty"`
	Id             uint64 `json:"id,omitempty"`
	Timestampbegin uint64 `json:"timestamp-begin,omitempty"`
	Totalamount    uint32 `json:"total-amount,omitempty"`
}

// Deprecated: use Stateactivity instead
type Activity struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Activity    bool   `json:"activity"`
}

// Stateactivity is new datamodel type version of Activity
type Stateactivity struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
	Activity       bool   `json:"activity"`
}

// Deprecated: Use Statereason instead
type DetectedAnomaly struct {
	DetectedAnomaly string `json:"detectedAnomaly"`
	TimestampMs     uint64 `json:"timestamp_ms"`
}

// Statereason is the new datamodel type version of DetectedAnomaly
type Statereason struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
	Reason         string `json:"reason"`
}

// Deprecated: use Shiftadd instead for the new data model
type AddShift struct {
	TimestampMs    uint64 `json:"timestamp_ms"`
	TimestampMsEnd uint64 `json:"timestamp_ms_end"`
}

// Shiftadd is new datamodel version of AddShift
type Shiftadd struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
	Timestampend   uint64 `json:"timestamp-end"`
}

// Deprecated: use Shiftdelete instead for the new datamodel
type DeleteShift struct {
	TimeStampMs uint64 `json:"timestamp_ms"`
}

// Shiftdelete is new datamodel version of DeleteShift
type Shiftdelete struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
}

// Deprecated: use Jobadd instead for the new data model
type AddOrder struct {
	ProductId   string `json:"product_id"`
	OrderId     string `json:"order_id"`
	TargetUnits uint64 `json:"target_units"`
}

// Jobadd is new datamodel version of AddOrder
type Jobadd struct {
	ProductType  string `json:"product-type"`
	Jobid        string `json:"job-id"`
	Targetamount uint64 `json:"target-amount"`
}

// Deprecated: use Producttypeadd instead
type AddProduct struct {
	ProductId            string  `json:"product_id"`
	TimePerUnitInSeconds float64 `json:"time_per_unit_in_seconds"`
}

// Producttypeadd is new datamodel version of AddProduct
type Producttypeadd struct {
	ProductId          string  `json:"product-id"`
	Cycletimeinseconds float64 `json:"cycle-time-in-seconds"`
}

// Deprecated: use Jobstart instead with new datamodel
type StartOrder struct {
	OrderId     string `json:"order_id"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// Jobstart is new datamodel version of StartOrder
type Jobstart struct {
	Jobid          string `json:"job-id"`
	Timestampbegin uint64 `json:"timestamp-begin"`
}

// Deprecated: Use Jobend instead
type EndOrder struct {
	OrderId     string `json:"order_id"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// Jobend is new datamodel version of endOrder
type Jobend struct {
	TimestampEnd uint64 `json:"timestamp-end"`
	Jobid        string `json:"job-id"`
}

// Deprecated: Use Productoverwrite instead
type ModifyProducedPieces struct {
	Count       int32  `json:"count"`
	Scrap       int32  `json:"scrap"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

// Productoverwrite is new datamodel version of ModifyProducedPieces
type Productoverwrite struct {
	Timestampend   uint64 `json:"timestamp-end"`
	Id             uint64 `json:"id,omitempty"`
	Timestampbegin uint64 `json:"timestamp-begin,omitempty"`
	Totalamount    int32  `json:"total-amount,omitempty"`
	Scrap          int32  `json:"scrap,omitempty"`
}

// Deprecated: use Stateadd instead for new datamodel
type State struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	State       uint64 `json:"state"`
}

// Stateadd is new datamodel version of State
type Stateadd struct {
	Timestampbegin uint64 `json:"timestamp-begin"`
	State          uint64 `json:"state"`
}

// Deprecated: Use Stateoverwrite instead
type ModifyState struct {
	StartTimeStampMs uint64 `json:"timestamp_ms"`
	EndTimeStampMs   uint64 `json:"timestamp_ms_end"`
	NewState         uint32 `json:"new_state"`
}

// Stateoverwrite is the new datamodel version of ModifyState
type Stateoverwrite struct {
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
