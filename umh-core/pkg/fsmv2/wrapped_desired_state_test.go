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

package fsmv2_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// Compile-time assertions: WrappedDesiredState satisfies framework interfaces.
var _ fsmv2.DesiredState = &fsmv2.WrappedDesiredState[testConfig]{}
var _ fsmv2.ShutdownRequestable = &fsmv2.WrappedDesiredState[testConfig]{}
var _ config.ChildSpecProvider = &fsmv2.WrappedDesiredState[testConfig]{}

// testConfig is a minimal worker configuration for testing.
type testConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

var _ = Describe("WrappedDesiredState", func() {
	Describe("IsShutdownRequested", func() {
		It("returns false when not requested", func() {
			ds := &fsmv2.WrappedDesiredState[testConfig]{}
			Expect(ds.IsShutdownRequested()).To(BeFalse())
		})

		It("returns true when requested", func() {
			ds := &fsmv2.WrappedDesiredState[testConfig]{
				BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
			}
			Expect(ds.IsShutdownRequested()).To(BeTrue())
		})
	})

	Describe("SetShutdownRequested", func() {
		It("sets shutdown to true", func() {
			ds := &fsmv2.WrappedDesiredState[testConfig]{}
			Expect(ds.IsShutdownRequested()).To(BeFalse())

			ds.SetShutdownRequested(true)
			Expect(ds.IsShutdownRequested()).To(BeTrue())
		})

		It("sets shutdown back to false", func() {
			ds := &fsmv2.WrappedDesiredState[testConfig]{
				BaseDesiredState: config.BaseDesiredState{ShutdownRequested: true},
			}
			ds.SetShutdownRequested(false)
			Expect(ds.IsShutdownRequested()).To(BeFalse())
		})
	})

	Describe("Config field", func() {
		It("stores and returns the typed configuration", func() {
			cfg := testConfig{Host: "192.168.1.100", Port: 502}
			ds := &fsmv2.WrappedDesiredState[testConfig]{
				Config: cfg,
			}
			Expect(ds.Config.Host).To(Equal("192.168.1.100"))
			Expect(ds.Config.Port).To(Equal(502))
		})
	})

	Describe("JSON round-trip", func() {
		It("round-trips via JSON marshal/unmarshal", func() {
			original := &fsmv2.WrappedDesiredState[testConfig]{
				BaseDesiredState: config.BaseDesiredState{},
				Config:           testConfig{Host: "localhost", Port: 8080},
			}
			original.SetShutdownRequested(true)

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored fsmv2.WrappedDesiredState[testConfig]
			Expect(json.Unmarshal(data, &restored)).To(Succeed())
			Expect(restored.Config.Host).To(Equal("localhost"))
			Expect(restored.Config.Port).To(Equal(8080))
			Expect(restored.IsShutdownRequested()).To(BeTrue())
		})
	})

	Describe("GetChildrenSpecs", func() {
		It("returns nil when no children are specified", func() {
			ds := &fsmv2.WrappedDesiredState[testConfig]{}
			Expect(ds.GetChildrenSpecs()).To(BeNil())
		})

		It("returns the configured children specs", func() {
			specs := []config.ChildSpec{
				{Name: "child-1", WorkerType: "mqtt_client"},
				{Name: "child-2", WorkerType: "modbus_client"},
			}
			ds := &fsmv2.WrappedDesiredState[testConfig]{
				ChildrenSpecs: specs,
			}
			Expect(ds.GetChildrenSpecs()).To(HaveLen(2))
			Expect(ds.GetChildrenSpecs()[0].Name).To(Equal("child-1"))
			Expect(ds.GetChildrenSpecs()[1].WorkerType).To(Equal("modbus_client"))
		})
	})
})
