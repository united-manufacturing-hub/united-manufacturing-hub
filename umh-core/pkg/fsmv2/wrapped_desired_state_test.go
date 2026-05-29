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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

var _ = Describe("WrappedDesiredState", func() {
	type TestConfig struct {
		URL  string
		Port int
	}

	Describe("zero value behavior", func() {
		It("IsShutdownRequested returns false for zero value", func() {
			wds := &fsmv2.WrappedDesiredState[TestConfig]{}
			Expect(wds.IsShutdownRequested()).To(BeFalse())
		})

		It("GetChildrenSpecs returns nil for zero value", func() {
			wds := &fsmv2.WrappedDesiredState[TestConfig]{}
			Expect(wds.GetChildrenSpecs()).To(BeNil())
		})
	})

	Describe("IsShutdownRequested", func() {
		It("returns false when shutdown is not requested", func() {
			wds := &fsmv2.WrappedDesiredState[TestConfig]{}
			wds.SetShutdownRequested(false)
			Expect(wds.IsShutdownRequested()).To(BeFalse())
		})

		It("returns true after SetShutdownRequested(true)", func() {
			wds := &fsmv2.WrappedDesiredState[TestConfig]{}
			wds.SetShutdownRequested(true)
			Expect(wds.IsShutdownRequested()).To(BeTrue())
		})

		It("returns false after SetShutdownRequested(false) called after true", func() {
			wds := &fsmv2.WrappedDesiredState[TestConfig]{}
			wds.SetShutdownRequested(true)
			wds.SetShutdownRequested(false)
			Expect(wds.IsShutdownRequested()).To(BeFalse())
		})
	})

	Describe("Config field", func() {
		It("stores and returns typed config", func() {
			cfg := TestConfig{URL: "http://localhost", Port: 8080}
			wds := &fsmv2.WrappedDesiredState[TestConfig]{
				Config: cfg,
			}
			Expect(wds.Config.URL).To(Equal("http://localhost"))
			Expect(wds.Config.Port).To(Equal(8080))
		})
	})

	Describe("satisfies DesiredState interface", func() {
		It("compile-time check", func() {
			var _ fsmv2.DesiredState = &fsmv2.WrappedDesiredState[TestConfig]{}
		})

		It("satisfies ShutdownRequestable interface", func() {
			var _ fsmv2.ShutdownRequestable = &fsmv2.WrappedDesiredState[TestConfig]{}
		})
	})
})
