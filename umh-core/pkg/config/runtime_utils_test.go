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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

var _ = Describe("RuntimeUtils", func() {
	Describe("GenerateBridgedBy", func() {
		It("should generate correct bridged_by for protocol converter", func() {
			result := config.GenerateBridgedBy(config.ComponentTypeProtocolConverter, "test-node", "temp-sensor")
			Expect(result).To(Equal("protocol-converter_test-node_temp-sensor"))
		})

		It("should generate correct bridged_by for stream processor", func() {
			result := config.GenerateBridgedBy(config.ComponentTypeStreamProcessor, "test-node", "pump-sp")
			Expect(result).To(Equal("stream-processor_test-node_pump-sp"))
		})

		It("should handle empty node name by defaulting to unknown", func() {
			result := config.GenerateBridgedBy(config.ComponentTypeProtocolConverter, "", "sensor")
			Expect(result).To(Equal("protocol-converter_unknown_sensor"))
		})

		It("should sanitize special characters", func() {
			result := config.GenerateBridgedBy(config.ComponentTypeProtocolConverter, "test@node#1", "temp.sensor@2")
			Expect(result).To(Equal("protocol-converter_test-node-1_temp-sensor-2"))
		})

		It("should collapse multiple dashes", func() {
			result := config.GenerateBridgedBy(config.ComponentTypeStreamProcessor, "test---node", "pump----sp")
			Expect(result).To(Equal("stream-processor_test-node_pump-sp"))
		})

		It("should trim leading and trailing dashes", func() {
			result := config.GenerateBridgedBy(config.ComponentTypeProtocolConverter, "-test-node-", "-sensor-")
			Expect(result).To(Equal("protocol-converter_test-node_sensor"))
		})

		It("should handle complex special character combinations", func() {
			result := config.GenerateBridgedBy(config.ComponentTypeStreamProcessor, "test@#$%node", "pump!@#$%^&*()sp")
			Expect(result).To(Equal("stream-processor_test-node_pump-sp"))
		})

		It("should handle numeric characters correctly", func() {
			result := config.GenerateBridgedBy(config.ComponentTypeProtocolConverter, "node123", "sensor456")
			Expect(result).To(Equal("protocol-converter_node123_sensor456"))
		})

		It("should handle mixed case correctly", func() {
			result := config.GenerateBridgedBy(config.ComponentTypeProtocolConverter, "TestNode", "TempSensor")
			Expect(result).To(Equal("protocol-converter_TestNode_TempSensor"))
		})
	})
})
