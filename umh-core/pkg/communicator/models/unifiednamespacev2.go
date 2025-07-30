// Copyright 2025 UMH Systems GmbH
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

package models

import (
	"encoding/hex"
	"time"

	"github.com/cespare/xxhash/v2"
)

type UnsElement interface {
	GetName() string
}

type MessageInfo struct {
	StartTime time.Time
	Count     int
}

// EventKafka represents Kafka-specific information of an Event, child of Event.
type EventKafka struct {
	Topic       string            `json:"topic"`
	Key         string            `json:"key"`
	Headers     map[string]string `json:"headers"`
	LastPayload string            `json:"lastPayload"`
	// Deprecated: Checkout Origin in models.EventTableEntry instead.
	Origin []string `json:"origin"`
	// Deprecated: Will be replaced by Destination in models.EventTableEntry.
	Destination            []string `json:"destination"`
	MessagesPerMinute      int      `json:"messagesPerMinute"`
	KafkaInsertedTimestamp int64    `json:"kafkaInsertedTimestamp"`
}

// Event represents the information about an event, child of Schema.
type Event struct {
	Error string     `json:"error"`
	Name  string     `json:"name"`
	Data  EventData  `json:"data"`
	Kafka EventKafka `json:"kafka"`
}

type SchemaName string

// Schema definitions
const (
	SCHEMA_ERROR_NO_SCHEMA_PRESENT SchemaName = "ERROR_NO_SCHEMA" // Do not add this to the ValidSchemas array
	SCHEMA_ERROR_NO_ENTERPRISE     SchemaName = "ERROR_NO_ENTERPRISE"
	SCHEMA_ANALYTICS               SchemaName = "_analytics"
	SCHEMA_HISTORIAN               SchemaName = "_historian"
	SCHEMA_LOCAL                   SchemaName = "_local"
)

// ValidSchemas is an array of strings that represents the valid schemas.
var ValidSchemas = [...]SchemaName{SCHEMA_HISTORIAN, SCHEMA_ANALYTICS, SCHEMA_LOCAL}

type EventData struct {
	// The value's type MUST be a string, bool, int (float64 for JSON numbers), or nil
	Value       interface{} `json:"value"`
	TimestampMs int64       `json:"timestamp_ms"`
	// TODO: Add `metadata`, see: https://linear.app/united-manufacturing-hub/issue/ENG-756/add-metadata-field-to-historian-schema
}

// Schema represents the leaf node and contains additional information.
type Schema struct {
	Name   SchemaName `json:"name"`
	Events []Event    `json:"events"`
}

// OriginId represents an origin id and contains schemas.
type OriginId struct {
	Name    string   `json:"name"`
	Schemas []Schema `json:"schemas"`
}

func (o OriginId) GetName() string {
	return o.Name
}

func OriginIdFactory(name string) OriginId {
	return OriginId{
		Name:    name,
		Schemas: []Schema{},
	}
}

// WorkCell represents a work cell and contains origin ids, optionally schemas.
type WorkCellV2 struct {
	Name      string     `json:"name"`
	Schemas   []Schema   `json:"schemas"`
	OriginIds []OriginId `json:"originIds"`
	Errors    []string   `json:"errors"`
}

func (w WorkCellV2) GetName() string {
	return w.Name
}

func WorkCellFactory(name string) WorkCellV2 {
	return WorkCellV2{
		Name:      name,
		Schemas:   []Schema{},
		OriginIds: []OriginId{},
		Errors:    make([]string, 0),
	}
}

// Line represents a production line and contains work cells, optionally schemas.
type LineV2 struct {
	Name      string       `json:"name"`
	Schemas   []Schema     `json:"schemas"`
	WorkCells []WorkCellV2 `json:"workCells"`
	Errors    []string     `json:"errors"`
}

func (l LineV2) GetName() string {
	return l.Name
}

func LineFactory(name string) LineV2 {
	return LineV2{
		Name:      name,
		Schemas:   []Schema{},
		WorkCells: []WorkCellV2{},
		Errors:    make([]string, 0),
	}
}

// Area represents an area and contains lines, optionally schemas.
type AreaV2 struct {
	Name    string   `json:"name"`
	Schemas []Schema `json:"schemas"`
	Lines   []LineV2 `json:"lines"`
	Errors  []string `json:"errors"`
}

func (a AreaV2) GetName() string {
	return a.Name
}

func AreaFactory(name string) AreaV2 {
	return AreaV2{
		Name:    name,
		Schemas: []Schema{},
		Lines:   []LineV2{},
		Errors:  make([]string, 0),
	}
}

// Site represents a site and contains areas, optionally schemas.
type SiteV2 struct {
	Name    string   `json:"name"`
	Areas   []AreaV2 `json:"areas"`
	Schemas []Schema `json:"schemas"`
	Errors  []string `json:"errors"`
}

func (s SiteV2) GetName() string {
	return s.Name
}

func SiteFactory(name string) SiteV2 {
	return SiteV2{
		Name:    name,
		Areas:   []AreaV2{},
		Schemas: []Schema{},
		Errors:  make([]string, 0),
	}
}

// Enterprise represents a enterprise and contains sites, optionally schemas.
type EnterpriseV2 struct {
	Name    string   `json:"name"`
	Schemas []Schema `json:"schemas"`
	Sites   []SiteV2 `json:"sites"`
	Errors  []string `json:"errors"`
}

func (e EnterpriseV2) GetName() string {
	return e.Name
}

func EnterpriseFactory(name string) EnterpriseV2 {
	return EnterpriseV2{
		Name:    name,
		Schemas: []Schema{},
		Sites:   []SiteV2{},
		Errors:  make([]string, 0),
	}
}

// UnifiedNamespace represents the UNS in a tree structure, starting from the enterprises level.
type UnifiedNamespaceV2 struct {
	Prefix      string         `json:"prefix"`
	Enterprises []EnterpriseV2 `json:"enterprises"`
}

func (u *UnifiedNamespaceV2) Len() int {
	nodes := 0
	for _, enterprise := range u.Enterprises {
		for _, site := range enterprise.Sites {
			nodes += len(site.Schemas)
			for _, area := range site.Areas {
				nodes += len(area.Schemas)
				for _, line := range area.Lines {
					nodes += len(line.Schemas)
					for _, workCell := range line.WorkCells {
						nodes += len(workCell.Schemas)
						for _, originId := range workCell.OriginIds {
							nodes += len(originId.Schemas)
						}
					}
				}
			}
		}
	}
	return nodes
}

type UnsTable map[string]UnsTableEntry

type UnsTableEntry struct {
	Enterprise string `json:"enterprise"`
	Site       string `json:"site"`
	Area       string `json:"area"`
	Line       string `json:"line"`
	WorkCell   string `json:"workCell"`
	OriginId   string `json:"originId"`
	Schema     string `json:"schema"`
	EventGroup string `json:"eventGroup"`
	EventTag   string `json:"eventTag"`
	HasError   bool   `json:"hasError"`
}

type EventTable map[int]EventTableEntry

type EventTableEntry struct {
	Value           interface{} `json:"value"`
	Origin          *string     `json:"origin"`
	UnsTreeId       string      `json:"unsTreeId"`
	Error           string      `json:"error"`
	Bridges         []string    `json:"bridges"`
	RawKafkaMessage EventKafka  `json:"rawKafkaMessage"`
	TimestampMs     int64       `json:"timestamp_ms"`
}

func HashUNSTableEntry(enterpriseName, siteName, areaName, lineName, workCellName, originId, schemaName, eventGroup, eventName string) string {
	hasher := xxhash.New()
	_, _ = hasher.Write([]byte(enterpriseName))
	_, _ = hasher.Write([]byte(siteName))
	_, _ = hasher.Write([]byte(areaName))
	_, _ = hasher.Write([]byte(lineName))
	_, _ = hasher.Write([]byte(workCellName))
	_, _ = hasher.Write([]byte(originId))
	_, _ = hasher.Write([]byte(schemaName))
	_, _ = hasher.Write([]byte(eventGroup))
	_, _ = hasher.Write([]byte(eventName))
	return hex.EncodeToString(hasher.Sum(nil))
}
