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

package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	Describe("Clone", func() {
		It("should create a deep copy of the config", func() {
			original := FullConfig{
				Agent: AgentConfig{
					MetricsPort: 8080,
				},
				Services: []S6FSMConfig{
					{
						FSMInstanceConfig: FSMInstanceConfig{
							Name:            "test-service",
							DesiredFSMState: "running",
						},
						S6ServiceConfig: S6ServiceConfig{
							Command: []string{"echo", "hello"},
							Env: map[string]string{
								"TEST": "value",
							},
						},
					},
				},
				Benthos: []BenthosConfig{
					{
						FSMInstanceConfig: FSMInstanceConfig{
							Name:            "test-benthos",
							DesiredFSMState: "running",
						},
						BenthosServiceConfig: BenthosServiceConfig{
							MetricsPort: 9090,
							LogLevel:    "INFO",
							Input: map[string]interface{}{
								"type": "http_server",
							},
						},
					},
				},
			}

			// Create a clone
			clone := original.Clone()

			// Verify initial equality
			Expect(clone).To(Equal(original))

			// Modify clone values at different levels
			clone.Agent.MetricsPort = 9000
			clone.Services[0].Name = "modified-service"
			clone.Services[0].S6ServiceConfig.Env["NEW"] = "value2"
			clone.Benthos[0].BenthosServiceConfig.Input["type"] = "kafka"

			// Verify original is unchanged
			Expect(original.Agent.MetricsPort).To(Equal(8080))
			Expect(original.Services[0].Name).To(Equal("test-service"))
			Expect(original.Services[0].S6ServiceConfig.Env).To(HaveLen(1))
			Expect(original.Services[0].S6ServiceConfig.Env["TEST"]).To(Equal("value"))
			Expect(original.Benthos[0].BenthosServiceConfig.Input["type"]).To(Equal("http_server"))

			// Verify clone has new values
			Expect(clone.Agent.MetricsPort).To(Equal(9000))
			Expect(clone.Services[0].Name).To(Equal("modified-service"))
			Expect(clone.Services[0].S6ServiceConfig.Env).To(HaveLen(2))
			Expect(clone.Services[0].S6ServiceConfig.Env["NEW"]).To(Equal("value2"))
			Expect(clone.Benthos[0].BenthosServiceConfig.Input["type"]).To(Equal("kafka"))

			// Verify that the pointers are different
			Expect(clone.Services[0].S6ServiceConfig).NotTo(BeIdenticalTo(original.Services[0].S6ServiceConfig))
			Expect(clone.Benthos[0].BenthosServiceConfig).NotTo(BeIdenticalTo(original.Benthos[0].BenthosServiceConfig))
		})
	})
})
