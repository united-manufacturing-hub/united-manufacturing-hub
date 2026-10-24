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

package logger

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("throttledLogger", func() {
	var (
		logs *observer.ObservedLogs
		w    *throttledLogger
	)

	BeforeEach(func() {
		core, recorded := observer.New(zapcore.DebugLevel)
		logs = recorded
		w = &throttledLogger{
			entries:  make(map[string]*throttleEntry),
			interval: time.Hour,
			logger:   zap.New(core).Sugar(),
		}
	})

	It("emits the first occurrence, a suppression notice on the second, then throttles the rest", func() {
		for range 5 {
			w.log("k", "busy", zapcore.WarnLevel, false)
		}

		Expect(logs.All()).To(HaveLen(2))
		Expect(logs.All()[0].Message).To(Equal("busy"))
		Expect(logs.All()[1].Message).To(ContainSubstring("suppressing further occurrences"))
	})

	It("escalates to Error once the suppressed count exceeds the threshold", func() {
		w.log("k", "busy", zapcore.WarnLevel, true) // first emission
		for range DefaultEscalateCounts + 1 {
			w.log("k", "busy", zapcore.WarnLevel, true) // suppressed
		}

		// Interval elapsed with more than DefaultEscalateCounts suppressed: escalate.
		w.entries["k"].lastLogged = time.Now().Add(-2 * w.interval)
		w.log("k", "busy", zapcore.WarnLevel, true)

		last := logs.All()[len(logs.All())-1]
		Expect(last.Level).To(Equal(zapcore.ErrorLevel))
		Expect(last.Message).To(ContainSubstring("further occurrences"))
	})
})
