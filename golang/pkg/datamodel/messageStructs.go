//go:build kafka
// +build kafka

package datamodel

/*
 * Based on https://docs.umh.app/docs/concepts/mqtt/ (22.04.2022)
 */

type Count struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Count       uint64 `json:"count"`
}

type ScrapCount struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Scrap       uint64 `json:"scrap"`
}

type Barcode struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Barcode     string `json:"barcode"`
}

type Activity struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Activity    bool   `json:"activity"`
}

type DetectedAnomaly struct {
	TimestampMs     uint64 `json:"timestamp_ms"`
	DetectedAnomaly string `json:"detectedAnomaly"`
}

type AddShift struct {
	TimestampMs    uint64 `json:"timestamp_ms"`
	TimestampMsEnd uint64 `json:"timestamp_ms_end"`
}

type AddOrder struct {
	OrderId     string `json:"order_id"`
	TargetUnits uint64 `json:"target_units"`
}

type AddProduct struct {
	ProductId            string  `json:"product_id"`
	TimePerUnitInSeconds float64 `json:"time_per_unit_in_seconds"`
}

type StartOrder struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	OrderId     string `json:"order_id"`
}

type EndOrder struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	OrderId     string `json:"order_id"`
}

type ProductImage struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Image       Image  `json:"image"`
}
type Image struct {
	ImageId       string `json:"image_id"`
	ImageBytes    string `json:"image_bytes"`
	ImageHeight   uint64 `json:"image_height"`
	ImageWidth    uint64 `json:"image_width"`
	ImageChannels uint8  `json:"image_channels"`
}

type ProductTag struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	AID         string `json:"aid"`
	Name        string `json:"name"`
	Value       int64  `json:"value"`
}

type ProductTagString struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	AID         string `json:"aid"`
	Name        string `json:"name"`
	Value       string `json:"value"`
}

type AddParentToChild struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	ParentAID   string `json:"parentAID"`
	ChildAID    string `json:"childAID"`
}

type State struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	State       uint64 `json:"state"`
}

type CycleTimeTrigger struct {
	TimestampMs    uint64 `json:"timestamp_ms"`
	CurrentStation string `json:"currentStation"`
	LastStation    string `json:"lastStation"`
	SanityTimeInS  uint64 `json:"sanityTime_in_s"`
}

type UniqueProduct struct {
	BeginTimestampMs           uint64 `json:"begin_timestamp_ms"`
	EndTimestampMs             uint64 `json:"end_timestamp_ms"`
	ProductId                  string `json:"product_id"`
	IsScrap                    bool   `json:"is_scrap"`
	UniqueProductAlternativeId string `json:"UniqueProductAlternativeId"`
}

type ScrapUniqueProduct struct {
	UID string `json:"uid"`
}

type Recommendations struct {
	TimestampMs          uint64            `json:"timestamp_ms"`
	RecommendationUID    string            `json:"recommendationUID"`
	RecommendationType   string            `json:"recommendationType"`
	Enabled              bool              `json:"enabled"`
	RecommendationValues map[string]string `json:"recommendationValues"`
}
