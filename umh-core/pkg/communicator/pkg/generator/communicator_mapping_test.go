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

package generator

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/managementconsole"
)

// zeroFields lists the paths of all zero fields, descending into structs and pointers.
func zeroFields(value reflect.Value, path string) []string {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return []string{path}
		}
		return zeroFields(value.Elem(), path)
	}
	if value.Kind() != reflect.Struct {
		if value.IsZero() {
			return []string{path}
		}
		return nil
	}
	var zero []string
	for index := range value.NumField() {
		fieldPath := path + "." + value.Type().Field(index).Name
		zero = append(zero, zeroFields(value.Field(index), fieldPath)...)
	}
	return zero
}

var _ = Describe("communicatorFromStatus", func() {
	It("checks that a Status with every field set fills every field of the connection part", func() {
		status := &managementconsole.Status{
			Health:              managementconsole.Degraded,
			HealthMessage:       "planted health message",
			State:               "Degraded",
			ConsecutiveErrors:   4,
			LastErrorType:       "server_error",
			OutboundFillPercent: 12.5,
			OutboundPeakPercent: 40,
			Push: &managementconsole.ChannelStatus{
				State:             "Running",
				LastErrorType:     "push_error",
				Messages:          900,
				MessagesDropped:   7,
				PendingMessages:   2,
				ConsecutiveErrors: 1,
				LastStatusCode:    503,
				LastLatencyMs:     12.5,
				Backpressured:     true,
			},
			Pull: &managementconsole.ChannelStatus{
				State:             "Degraded",
				LastErrorType:     "pull_error",
				Messages:          800,
				MessagesDropped:   6,
				PendingMessages:   5,
				ConsecutiveErrors: 8,
				LastStatusCode:    429,
				LastLatencyMs:     30,
				Backpressured:     true,
			},
		}

		communicator := communicatorFromStatus(status, 3)

		Expect(zeroFields(reflect.ValueOf(communicator).Elem(), "Communicator")).To(BeEmpty(),
			"every field of the connection part must be filled from the Status or from umh-core")
	})
})
