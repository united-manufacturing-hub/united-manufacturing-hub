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

// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package graphql

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"time"
)

type Event interface {
	IsEvent()
	GetProducedAt() time.Time
	GetHeaders() []*MetadataKv
}

type MetaExpr struct {
	Key string  `json:"key"`
	Eq  *string `json:"eq,omitempty"`
}

type MetadataKv struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Query struct {
}

type RelationalEvent struct {
	ProducedAt time.Time      `json:"producedAt"`
	Headers    []*MetadataKv  `json:"headers"`
	JSON       map[string]any `json:"json"`
}

func (RelationalEvent) IsEvent()                      {}
func (this RelationalEvent) GetProducedAt() time.Time { return this.ProducedAt }
func (this RelationalEvent) GetHeaders() []*MetadataKv {
	if this.Headers == nil {
		return nil
	}
	interfaceSlice := make([]*MetadataKv, 0, len(this.Headers))
	for _, concrete := range this.Headers {
		interfaceSlice = append(interfaceSlice, concrete)
	}
	return interfaceSlice
}

type TimeSeriesEvent struct {
	ProducedAt   time.Time     `json:"producedAt"`
	Headers      []*MetadataKv `json:"headers"`
	SourceTs     time.Time     `json:"sourceTs"`
	ScalarType   ScalarType    `json:"scalarType"`
	NumericValue *float64      `json:"numericValue,omitempty"`
	StringValue  *string       `json:"stringValue,omitempty"`
	BooleanValue *bool         `json:"booleanValue,omitempty"`
}

func (TimeSeriesEvent) IsEvent()                      {}
func (this TimeSeriesEvent) GetProducedAt() time.Time { return this.ProducedAt }
func (this TimeSeriesEvent) GetHeaders() []*MetadataKv {
	if this.Headers == nil {
		return nil
	}
	interfaceSlice := make([]*MetadataKv, 0, len(this.Headers))
	for _, concrete := range this.Headers {
		interfaceSlice = append(interfaceSlice, concrete)
	}
	return interfaceSlice
}

type Topic struct {
	Topic     string        `json:"topic"`
	Metadata  []*MetadataKv `json:"metadata"`
	LastEvent Event         `json:"lastEvent,omitempty"`
}

type TopicFilter struct {
	Text *string     `json:"text,omitempty"`
	Meta []*MetaExpr `json:"meta,omitempty"`
}

type ScalarType string

const (
	ScalarTypeNumeric ScalarType = "NUMERIC"
	ScalarTypeString  ScalarType = "STRING"
	ScalarTypeBoolean ScalarType = "BOOLEAN"
)

var AllScalarType = []ScalarType{
	ScalarTypeNumeric,
	ScalarTypeString,
	ScalarTypeBoolean,
}

func (e ScalarType) IsValid() bool {
	switch e {
	case ScalarTypeNumeric, ScalarTypeString, ScalarTypeBoolean:
		return true
	}
	return false
}

func (e ScalarType) String() string {
	return string(e)
}

func (e *ScalarType) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = ScalarType(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid ScalarType", str)
	}
	return nil
}

func (e ScalarType) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

func (e *ScalarType) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	return e.UnmarshalGQL(s)
}

func (e ScalarType) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	e.MarshalGQL(&buf)
	return buf.Bytes(), nil
}
