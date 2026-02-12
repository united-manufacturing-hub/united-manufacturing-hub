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

// Package panicutil provides shared panic classification utilities for the supervisor hierarchy.
package panicutil

import (
	"errors"
	"fmt"
)

const (
	PanicTypeError   = "error_panic"
	PanicTypeString  = "string_panic"
	PanicTypeUnknown = "unknown_panic"
)

// ClassifyPanic converts a recovered panic value into a typed error and a classification string.
func ClassifyPanic(r interface{}) (panicType string, panicErr error) {
	switch v := r.(type) {
	case error:
		return PanicTypeError, v
	case string:
		return PanicTypeString, errors.New(v)
	default:
		return PanicTypeUnknown, fmt.Errorf("%v", r)
	}
}
