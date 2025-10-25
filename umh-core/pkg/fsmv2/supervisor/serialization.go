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

package supervisor

import (
	"fmt"
	"reflect"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

func ToDocument(v interface{}) (basic.Document, error) {
	var doc basic.Document

	config := &mapstructure.DecoderConfig{
		Result:           &doc,
		WeaklyTypedInput: true,
		TagName:          "json",
		DecodeHook:       preserveTimeHook(),
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(v); err != nil {
		return nil, fmt.Errorf("failed to encode to document: %w", err)
	}

	return doc, nil
}

func FromDocument(doc basic.Document, targetType reflect.Type) (interface{}, error) {
	result := reflect.New(targetType).Interface()

	config := &mapstructure.DecoderConfig{
		Result:           result,
		WeaklyTypedInput: true,
		TagName:          "json",
		DecodeHook:       mapstructure.StringToTimeHookFunc(time.RFC3339),
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(doc); err != nil {
		return nil, fmt.Errorf("failed to decode from document: %w", err)
	}

	return reflect.ValueOf(result).Elem().Interface(), nil
}

func preserveTimeHook() mapstructure.DecodeHookFunc {
	timeType := reflect.TypeOf(time.Time{})
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f == timeType || (f.Kind() == reflect.Ptr && f.Elem() == timeType) {
			val := reflect.ValueOf(data)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}
			return val.Interface(), nil
		}
		return data, nil
	}
}
