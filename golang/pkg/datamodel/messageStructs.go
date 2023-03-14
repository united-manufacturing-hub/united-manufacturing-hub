// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	Barcode     string `json:"barcode"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

type Activity struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Activity    bool   `json:"activity"`
}

type DetectedAnomaly struct {
	DetectedAnomaly string `json:"detectedAnomaly"`
	TimestampMs     uint64 `json:"timestamp_ms"`
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
	OrderId     string `json:"order_id"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

type EndOrder struct {
	OrderId     string `json:"order_id"`
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

type ProductTag struct {
	AID         string `json:"aid"`
	Name        string `json:"name"`
	TimestampMs uint64 `json:"timestamp_ms"`
	Value       int64  `json:"value"`
}

type ProductTagString struct {
	AID         string `json:"aid"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

type AddParentToChild struct {
	ParentAID   string `json:"parentAID"`
	ChildAID    string `json:"childAID"`
	TimestampMs uint64 `json:"timestamp_ms"`
}

type State struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	State       uint64 `json:"state"`
}

type CycleTimeTrigger struct {
	CurrentStation string `json:"currentStation"`
	LastStation    string `json:"lastStation"`
	TimestampMs    uint64 `json:"timestamp_ms"`
	SanityTimeInS  uint64 `json:"sanityTime_in_s"`
}

type UniqueProduct struct {
	ProductId                  string `json:"product_id"`
	UniqueProductAlternativeId string `json:"UniqueProductAlternativeId"`
	BeginTimestampMs           uint64 `json:"begin_timestamp_ms"`
	EndTimestampMs             uint64 `json:"end_timestamp_ms"`
	IsScrap                    bool   `json:"is_scrap"`
}

type ScrapUniqueProduct struct {
	UID string `json:"uid"`
}

type Recommendations struct {
	RecommendationValues map[string]string `json:"recommendationValues"`
	RecommendationUID    string            `json:"recommendationUID"`
	RecommendationType   string            `json:"recommendationType"`
	TimestampMs          uint64            `json:"timestamp_ms"`
	Enabled              bool              `json:"enabled"`
}
