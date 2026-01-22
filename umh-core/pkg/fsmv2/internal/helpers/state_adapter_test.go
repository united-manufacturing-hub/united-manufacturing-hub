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

package helpers_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
)

// Test types for the state adapter tests.
type testObserved struct {
	Value     string
	Timestamp time.Time
}

func (t testObserved) GetObservedDesiredState() fsmv2.DesiredState {
	return &testDesired{}
}

func (t testObserved) GetTimestamp() time.Time {
	return t.Timestamp
}

type testDesired struct {
	shutdown bool
}

func (t *testDesired) IsShutdownRequested() bool {
	return t.shutdown
}

func (t *testDesired) GetState() string {
	return "running"
}

type wrongObserved struct {
	Value string
}

func (w wrongObserved) GetObservedDesiredState() fsmv2.DesiredState {
	return &testDesired{}
}

func (w wrongObserved) GetTimestamp() time.Time {
	return time.Now()
}

type wrongDesired struct {
	value int
}

func (w *wrongDesired) IsShutdownRequested() bool {
	return false
}

func (w *wrongDesired) GetState() string {
	return "running"
}

var _ = Describe("StateAdapter", func() {
	Describe("ConvertSnapshot", func() {
		Context("successful conversion", func() {
			It("should convert snapshot with correct types", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				observed := testObserved{
					Value:     "observed-value",
					Timestamp: time.Now(),
				}
				desired := &testDesired{shutdown: true}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: observed,
					Desired:  desired,
				}

				typedSnap := helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)

				Expect(typedSnap.Identity.ID).To(Equal(identity.ID))
				Expect(typedSnap.Identity.Name).To(Equal(identity.Name))
				Expect(typedSnap.Identity.WorkerType).To(Equal(identity.WorkerType))
				Expect(typedSnap.Observed.Value).To(Equal(observed.Value))
				Expect(typedSnap.Desired.IsShutdownRequested()).To(BeTrue())
			})

			It("should handle value type observed", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				observed := testObserved{
					Value:     "value-type-test",
					Timestamp: time.Now(),
				}
				desired := &testDesired{shutdown: false}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: observed,
					Desired:  desired,
				}

				typedSnap := helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)

				Expect(typedSnap.Observed.Value).To(Equal("value-type-test"))
			})

			It("should handle pointer type desired", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				observed := testObserved{
					Value:     "test",
					Timestamp: time.Now(),
				}
				desired := &testDesired{shutdown: true}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: observed,
					Desired:  desired,
				}

				typedSnap := helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)

				Expect(typedSnap.Desired).NotTo(BeNil())
				Expect(typedSnap.Desired.IsShutdownRequested()).To(BeTrue())
			})
		})

		Context("panic scenarios", func() {
			It("should panic on wrong observed type", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				observed := wrongObserved{Value: "wrong"}
				desired := &testDesired{shutdown: false}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: observed,
					Desired:  desired,
				}

				Expect(func() {
					helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
				}).To(Panic())
			})

			It("should panic with message mentioning Observed", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				observed := wrongObserved{Value: "wrong"}
				desired := &testDesired{shutdown: false}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: observed,
					Desired:  desired,
				}

				defer func() {
					r := recover()
					Expect(r).NotTo(BeNil())
					panicMsg, ok := r.(string)
					Expect(ok).To(BeTrue())
					Expect(panicMsg).To(ContainSubstring("Observed"))
				}()

				helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
			})

			It("should panic on wrong desired type", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				observed := testObserved{Value: "test", Timestamp: time.Now()}
				desired := &wrongDesired{value: 42}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: observed,
					Desired:  desired,
				}

				Expect(func() {
					helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
				}).To(Panic())
			})

			It("should panic with message mentioning Desired", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				observed := testObserved{Value: "test", Timestamp: time.Now()}
				desired := &wrongDesired{value: 42}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: observed,
					Desired:  desired,
				}

				defer func() {
					r := recover()
					Expect(r).NotTo(BeNil())
					panicMsg, ok := r.(string)
					Expect(ok).To(BeTrue())
					Expect(panicMsg).To(ContainSubstring("Desired"))
				}()

				helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
			})

			It("should panic on non-Snapshot input", func() {
				notASnapshot := "not a snapshot"

				Expect(func() {
					helpers.ConvertSnapshot[testObserved, *testDesired](notASnapshot)
				}).To(Panic())
			})

			It("should panic with message mentioning Snapshot for non-Snapshot input", func() {
				notASnapshot := "not a snapshot"

				defer func() {
					r := recover()
					Expect(r).NotTo(BeNil())
					panicMsg, ok := r.(string)
					Expect(ok).To(BeTrue())
					Expect(panicMsg).To(ContainSubstring("Snapshot"))
				}()

				helpers.ConvertSnapshot[testObserved, *testDesired](notASnapshot)
			})

			It("should panic on nil observed", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				desired := &testDesired{shutdown: false}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: nil,
					Desired:  desired,
				}

				Expect(func() {
					helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
				}).To(Panic())
			})

			It("should panic with message mentioning Observed for nil observed", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				desired := &testDesired{shutdown: false}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: nil,
					Desired:  desired,
				}

				defer func() {
					r := recover()
					Expect(r).NotTo(BeNil())
					panicMsg, ok := r.(string)
					Expect(ok).To(BeTrue())
					Expect(panicMsg).To(ContainSubstring("Observed"))
				}()

				helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
			})

			It("should panic on nil desired", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				observed := testObserved{Value: "test", Timestamp: time.Now()}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: observed,
					Desired:  nil,
				}

				Expect(func() {
					helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
				}).To(Panic())
			})

			It("should panic with message mentioning Desired for nil desired", func() {
				identity := fsmv2.Identity{
					ID:         "test-id",
					Name:       "test-name",
					WorkerType: "test-worker",
				}
				observed := testObserved{Value: "test", Timestamp: time.Now()}

				rawSnapshot := fsmv2.Snapshot{
					Identity: identity,
					Observed: observed,
					Desired:  nil,
				}

				defer func() {
					r := recover()
					Expect(r).NotTo(BeNil())
					panicMsg, ok := r.(string)
					Expect(ok).To(BeTrue())
					Expect(panicMsg).To(ContainSubstring("Desired"))
				}()

				helpers.ConvertSnapshot[testObserved, *testDesired](rawSnapshot)
			})
		})
	})

	Describe("TypedSnapshot", func() {
		It("should support direct field access", func() {
			identity := fsmv2.Identity{
				ID:         "test-id",
				Name:       "test-name",
				WorkerType: "test-worker",
			}
			observed := testObserved{
				Value:     "direct-access-test",
				Timestamp: time.Now(),
			}
			desired := &testDesired{shutdown: true}

			typedSnap := helpers.TypedSnapshot[testObserved, *testDesired]{
				Identity: identity,
				Observed: observed,
				Desired:  desired,
			}

			Expect(typedSnap.Identity.ID).To(Equal("test-id"))
			Expect(typedSnap.Observed.Value).To(Equal("direct-access-test"))
			Expect(typedSnap.Desired.IsShutdownRequested()).To(BeTrue())
		})
	})
})

// containsSubstring is a helper function to check if a string contains a substring.
func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}
