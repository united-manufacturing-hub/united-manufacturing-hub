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

package sentry_test

import (
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry"

	//nolint:revive // dot import for Ginkgo DSL
	. "github.com/onsi/ginkgo/v2"
	//nolint:revive // dot import for Gomega matchers
	. "github.com/onsi/gomega"
)

var _ = Describe("ErrorFields", func() {

	Describe("ZapFields", func() {
		It("should return correct slice with feature and error keys", func() {
			fields := sentry.ErrorFields{
				Feature: "communicator",
				Err:     errors.New("test"),
			}

			result := fields.ZapFields()

			Expect(result).To(HaveLen(4))
			Expect(result[0]).To(Equal("feature"))
			Expect(result[1]).To(Equal("communicator"))
			Expect(result[2]).To(Equal("error"))
			Expect(result[3]).To(BeAssignableToTypeOf(errors.New("")))
		})

		It("should accept error interface type (compile-time type safety)", func() {
			// This test verifies that Err field is of type error, not string.
			// If this compiles and runs, the type safety is enforced.
			err := errors.New("typed error")
			fields := sentry.ErrorFields{
				Feature: "test-feature",
				Err:     err,
			}

			result := fields.ZapFields()

			Expect(result).To(ContainElement("error"))
			// The error should be stored as error interface, not converted to string
			for i, v := range result {
				if v == "error" && i+1 < len(result) {
					_, isError := result[i+1].(error)
					Expect(isError).To(BeTrue(), "Err field should be stored as error interface, not string")
				}
			}
		})

		It("should return fields without error key when Err is nil", func() {
			fields := sentry.ErrorFields{
				Feature: "communicator",
				Err:     nil,
			}

			result := fields.ZapFields()

			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(Equal("feature"))
			Expect(result[1]).To(Equal("communicator"))
			Expect(result).NotTo(ContainElement("error"))
		})

		It("should include HierarchyPath when provided", func() {
			fields := sentry.ErrorFields{
				Feature:       "fsm",
				Err:           errors.New("state error"),
				HierarchyPath: "app(application)/worker(communicator)",
			}

			result := fields.ZapFields()

			Expect(result).To(ContainElement("hierarchy_path"))
			Expect(result).To(ContainElement("app(application)/worker(communicator)"))
		})

		It("should include WorkerID when provided", func() {
			fields := sentry.ErrorFields{
				Feature:  "worker",
				Err:      errors.New("worker error"),
				WorkerID: "worker-123",
			}

			result := fields.ZapFields()

			Expect(result).To(ContainElement("worker_id"))
			Expect(result).To(ContainElement("worker-123"))
		})

		It("should include WorkerType when provided", func() {
			fields := sentry.ErrorFields{
				Feature:    "worker",
				Err:        errors.New("worker error"),
				WorkerType: "communicator",
			}

			result := fields.ZapFields()

			Expect(result).To(ContainElement("worker_type"))
			Expect(result).To(ContainElement("communicator"))
		})

		It("should include all optional fields when provided", func() {
			fields := sentry.ErrorFields{
				Feature:       "control-loop",
				Err:           errors.New("comprehensive error"),
				HierarchyPath: "app(test)/worker(processor)",
				WorkerID:      "worker-456",
				WorkerType:    "processor",
			}

			result := fields.ZapFields()

			// Should have: feature, error, hierarchy_path, worker_id, worker_type (5 pairs = 10 elements)
			Expect(result).To(HaveLen(10))
			Expect(result).To(ContainElement("feature"))
			Expect(result).To(ContainElement("control-loop"))
			Expect(result).To(ContainElement("error"))
			Expect(result).To(ContainElement("hierarchy_path"))
			Expect(result).To(ContainElement("app(test)/worker(processor)"))
			Expect(result).To(ContainElement("worker_id"))
			Expect(result).To(ContainElement("worker-456"))
			Expect(result).To(ContainElement("worker_type"))
			Expect(result).To(ContainElement("processor"))
		})

		It("should omit empty optional fields", func() {
			fields := sentry.ErrorFields{
				Feature:       "minimal",
				Err:           errors.New("minimal error"),
				HierarchyPath: "", // empty, should be omitted
				WorkerID:      "", // empty, should be omitted
				WorkerType:    "", // empty, should be omitted
			}

			result := fields.ZapFields()

			// Should only have: feature, error (2 pairs = 4 elements)
			Expect(result).To(HaveLen(4))
			Expect(result).NotTo(ContainElement("hierarchy_path"))
			Expect(result).NotTo(ContainElement("worker_id"))
			Expect(result).NotTo(ContainElement("worker_type"))
		})
	})
})
