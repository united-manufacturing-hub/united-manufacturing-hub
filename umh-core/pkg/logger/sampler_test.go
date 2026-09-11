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
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestLogger(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logger Suite")
}

// countingCore records how many entries were written, per level.
type countingCore struct {
	mu     sync.Mutex
	counts map[zapcore.Level]int
}

func newCountingCore() *countingCore { return &countingCore{counts: map[zapcore.Level]int{}} }

func (c *countingCore) Enabled(zapcore.Level) bool { return true }
func (c *countingCore) With([]zapcore.Field) zapcore.Core { return c }
func (c *countingCore) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(e, c)
}
func (c *countingCore) Write(e zapcore.Entry, _ []zapcore.Field) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[e.Level]++
	return nil
}
func (c *countingCore) Sync() error { return nil }
func (c *countingCore) get(l zapcore.Level) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[l]
}

var _ = Describe("NewLevelSampledCore", func() {
	It("samples below Warn but never drops Warn or Error", func() {
		inner := newCountingCore()
		// first=3, thereafter=100, huge tick so everything lands in one window.
		log := zap.New(NewLevelSampledCore(inner, time.Hour, 3, 100))

		const n = 250
		for range n {
			log.Info("same info message")
			log.Warn("same warn message")
			log.Error("same error message")
		}

		// Info below Warn: first 3 logged, then every 100th -> 3 + floor((250-3)/100) = 5.
		Expect(inner.get(zapcore.InfoLevel)).To(Equal(5))
		// Warn and Error must never be sampled.
		Expect(inner.get(zapcore.WarnLevel)).To(Equal(n))
		Expect(inner.get(zapcore.ErrorLevel)).To(Equal(n))
	})

	It("does not group distinct messages into one bucket", func() {
		inner := newCountingCore()
		log := zap.New(NewLevelSampledCore(inner, time.Hour, 1, 100))

		// Distinct messages each get their own bucket -> each logged once.
		log.Info("message A")
		log.Info("message B")
		log.Info("message A")

		Expect(inner.get(zapcore.InfoLevel)).To(Equal(2))
	})
})
