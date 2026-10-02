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
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// unmarshalableObservedState carries a channel, which json.Marshal cannot
// encode, so converting it always fails at the marshal stage.
type unmarshalableObservedState struct {
	Channel chan int
}

func (unmarshalableObservedState) GetTimestamp() time.Time { return time.Time{} }

// sentryErrorRecorder captures the Sentry events a supervisor reports.
type sentryErrorRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *sentryErrorRecorder) Debug(_ string, _ ...deps.Field) {}
func (r *sentryErrorRecorder) Info(_ string, _ ...deps.Field)  {}
func (r *sentryErrorRecorder) SentryWarn(_ deps.Feature, _ string, _ string, _ ...deps.Field) {
}

func (r *sentryErrorRecorder) SentryError(_ deps.Feature, _ string, _ error, msg string, _ ...deps.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, msg)
}

func (r *sentryErrorRecorder) With(_ ...deps.Field) deps.FSMLogger { return r }

func (r *sentryErrorRecorder) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string{}, r.events...)
}

var _ = Describe("saveInitialState marshal failure reporting", func() {
	It("reports worker_add_marshal_observed_failed and wraps with the saveInitialState error text", func() {
		rec := &sentryErrorRecorder{}
		s := &Supervisor[*TestObservedState, *TestDesiredState]{
			logger:     rec,
			workerType: "test",
			store:      CreateTestTriangularStoreForWorkerType("test"),
		}

		observed := unmarshalableObservedState{Channel: make(chan int)}

		err := s.saveInitialState(context.Background(), nil, deps.Identity{ID: "w1", WorkerType: "test"}, observed, &TestDesiredState{}, 1)

		Expect(err).To(MatchError(ContainSubstring(
			"failed to marshal observed state: json: unsupported type: chan int")))
		Expect(rec.Events()).To(ConsistOf("worker_add_marshal_observed_failed"))
	})
})

var _ = Describe("toDocument", func() {
	It("reports the event even when the worker has an empty hierarchy path", func() {
		rec := &sentryErrorRecorder{}
		s := &Supervisor[*TestObservedState, *TestDesiredState]{logger: rec}

		observed := unmarshalableObservedState{Channel: make(chan int)}

		Expect(s.toDocument(observed, "w2", "", documentConversion{
			marshalEvent:     "worker_add_marshal_observed_failed",
			marshalErrPrefix: "failed to marshal observed state",
		})).Error().To(HaveOccurred())

		Expect(rec.Events()).To(ConsistOf("worker_add_marshal_observed_failed"))
	})

	It("reports nothing when the conversion carries no events", func() {
		rec := &sentryErrorRecorder{}
		s := &Supervisor[*TestObservedState, *TestDesiredState]{logger: rec}

		observed := unmarshalableObservedState{Channel: make(chan int)}

		_, err := s.toDocument(observed, "w3", "", documentConversion{})
		Expect(err).To(HaveOccurred(),
			"the spec needs a failing conversion: an empty event skips only the Sentry report, not the error")
		Expect(rec.Events()).To(BeEmpty(),
			"an empty event must skip the Sentry report")
	})

	It("sets the id field", func() {
		rec := &sentryErrorRecorder{}
		s := &Supervisor[*TestObservedState, *TestDesiredState]{logger: rec}

		doc, err := s.toDocument(map[string]any{"a": 1}, "w4", "", documentConversion{})
		Expect(err).NotTo(HaveOccurred())
		Expect(doc[FieldID]).To(Equal("w4"))
	})
})
