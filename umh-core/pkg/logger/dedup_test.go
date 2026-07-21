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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DedupLogger", func() {
	var (
		logs  *observer.ObservedLogs
		dedup *DedupLogger
	)

	BeforeEach(func() {
		core, recorded := observer.New(zapcore.DebugLevel)
		logs = recorded
		dedup = NewDedupLogger(zap.New(core).Sugar())
	})

	levels := func() []zapcore.Level {
		out := make([]zapcore.Level, 0)
		for _, e := range logs.All() {
			out = append(out, e.Level)
		}
		return out
	}

	It("logs first occurrence and first repeat at Error, then demotes to Debug", func() {
		for range 4 {
			dedup.LogErrorDedup("boom %d", 1)
		}

		Expect(levels()).To(Equal([]zapcore.Level{
			zapcore.ErrorLevel, // first occurrence
			zapcore.ErrorLevel, // first repeat (suppression notice)
			zapcore.DebugLevel, // further repeats
			zapcore.DebugLevel,
		}))
	})

	It("does not restart the cycle when Reset is called between identical messages", func() {
		// Simulates an FSM reconcile loop that reports success (Reset) every tick
		// while the same error keeps being logged from a swallowed path.
		dedup.LogErrorDedup("still broken")
		dedup.Reset()
		dedup.LogErrorDedup("still broken")
		dedup.Reset()
		dedup.LogErrorDedup("still broken")

		// Only the first two are Error; the spurious Resets must not re-promote.
		Expect(levels()).To(Equal([]zapcore.Level{
			zapcore.ErrorLevel,
			zapcore.ErrorLevel,
			zapcore.DebugLevel,
		}))
	})

	It("starts a fresh Error cycle when the message changes after a Reset", func() {
		dedup.LogErrorDedup("error A")
		dedup.Reset()
		dedup.LogErrorDedup("error B")

		Expect(levels()).To(Equal([]zapcore.Level{
			zapcore.ErrorLevel, // A
			zapcore.ErrorLevel, // B (changed message ends prior cycle, logs fresh)
		}))
	})
})
