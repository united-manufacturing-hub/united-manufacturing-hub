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

package deps_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

func parseLine(buf *bytes.Buffer) map[string]interface{} {
	line := strings.TrimSpace(buf.String())
	if line == "" {
		return nil
	}
	lines := strings.Split(line, "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	m := make(map[string]interface{})
	ExpectWithOffset(1, json.Unmarshal([]byte(last), &m)).To(Succeed())
	return m
}

var _ = Describe("zapLogger field propagation", func() {
	var buf *bytes.Buffer

	BeforeEach(func() {
		buf = new(bytes.Buffer)
	})

	It("SentryError produces JSON with feature, error, and msg fields", func() {
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		testErr := errors.New("something broke")
		logger.SentryError(deps.FeatureFSMv2, "root/worker-1(helloworld)", testErr, "lifecycle failed")

		m := parseLine(buf)
		Expect(m).To(HaveKeyWithValue("msg", "lifecycle failed"))
		Expect(m).To(HaveKeyWithValue("feature", "fsmv2"))
		Expect(m).To(HaveKeyWithValue("hierarchy_path", "root/worker-1(helloworld)"))
		Expect(m).To(HaveKey("error"))
		Expect(m).To(HaveKeyWithValue("level", "error"))
	})

	It("SentryWarn produces JSON with feature and msg fields", func() {
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		logger.SentryWarn(deps.FeatureFSMv2, "root/worker-1(helloworld)", "reconciliation slow")

		m := parseLine(buf)
		Expect(m).To(HaveKeyWithValue("msg", "reconciliation slow"))
		Expect(m).To(HaveKeyWithValue("feature", "fsmv2"))
		Expect(m).To(HaveKeyWithValue("hierarchy_path", "root/worker-1(helloworld)"))
		Expect(m).To(HaveKeyWithValue("level", "warn"))
	})

	It("SentryError omits hierarchy_path when empty", func() {
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		testErr := errors.New("something broke")
		logger.SentryError(deps.FeatureFSMv2, "", testErr, "lifecycle failed")

		m := parseLine(buf)
		Expect(m).NotTo(HaveKey("hierarchy_path"))
	})

	It("SentryWarn omits hierarchy_path when empty", func() {
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		logger.SentryWarn(deps.FeatureFSMv2, "", "reconciliation slow")

		m := parseLine(buf)
		Expect(m).NotTo(HaveKey("hierarchy_path"))
	})

	It("Info produces JSON with msg field at info level", func() {
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		logger.Info("hello info")

		m := parseLine(buf)
		Expect(m).To(HaveKeyWithValue("msg", "hello info"))
		Expect(m).To(HaveKeyWithValue("level", "info"))
	})

	It("Debug produces JSON with msg field at debug level", func() {
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		logger.Debug("debug trace")

		m := parseLine(buf)
		Expect(m).To(HaveKeyWithValue("msg", "debug trace"))
		Expect(m).To(HaveKeyWithValue("level", "debug"))
	})
})

var _ = Describe("NewFSMLogger nil panic guard", func() {
	It("panics with a message containing nil when sugar is nil", func() {
		Expect(func() {
			deps.NewFSMLogger(nil)
		}).To(PanicWith(ContainSubstring("nil")))
	})
})

var _ = Describe("With() chained field accumulation", func() {
	var buf *bytes.Buffer

	BeforeEach(func() {
		buf = new(bytes.Buffer)
	})

	It("logger.With(A).Info includes field A in output", func() {
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		child := logger.With(deps.WorkerID("w-1"))
		child.Info("with one field")

		m := parseLine(buf)
		Expect(m).To(HaveKeyWithValue("msg", "with one field"))
		Expect(m).To(HaveKeyWithValue("worker_id", "w-1"))
	})

	It("logger.With(A).With(B).Info includes both A and B", func() {
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		child := logger.With(deps.WorkerID("w-1")).With(deps.WorkerType("reconciler"))
		child.Info("with two fields")

		m := parseLine(buf)
		Expect(m).To(HaveKeyWithValue("msg", "with two fields"))
		Expect(m).To(HaveKeyWithValue("worker_id", "w-1"))
		Expect(m).To(HaveKeyWithValue("worker_type", "reconciler"))
	})
})

var _ = Describe("nopLogger contract", func() {
	It("all methods can be called without panic", func() {
		logger := deps.NewNopFSMLogger()
		testErr := errors.New("nop error")

		Expect(func() {
			logger.Debug("debug")
			logger.Info("info")
			logger.SentryWarn(deps.FeatureExamples, "", "warn")
			logger.SentryWarn(deps.FeatureFSMv2, "", "health warn")
			logger.SentryError(deps.FeatureFSMv2, "", testErr, "action error")
		}).NotTo(Panic())
	})

	It("With() returns a valid FSMLogger", func() {
		logger := deps.NewNopFSMLogger()
		child := logger.With(deps.WorkerID("nop-child"))

		Expect(child).NotTo(BeNil())

		var iface deps.FSMLogger = child
		Expect(iface).NotTo(BeNil())

		Expect(func() {
			child.Info("still works")
		}).NotTo(Panic())
	})
})

var _ = Describe("Field constructor key correctness", func() {
	It("HierarchyPath produces the correct key and value", func() {
		f := deps.HierarchyPath("x")
		Expect(f.Key).To(Equal("hierarchy_path"))
		Expect(f.Value).To(Equal("x"))
	})

	It("WorkerID produces the correct key and value", func() {
		f := deps.WorkerID("x")
		Expect(f.Key).To(Equal("worker_id"))
		Expect(f.Value).To(Equal("x"))
	})

	It("WorkerType produces the correct key and value", func() {
		f := deps.WorkerType("x")
		Expect(f.Key).To(Equal("worker_type"))
		Expect(f.Value).To(Equal("x"))
	})

	It("ActionName produces the correct key and value", func() {
		f := deps.ActionName("x")
		Expect(f.Key).To(Equal("action_name"))
		Expect(f.Value).To(Equal("x"))
	})

	It("CorrelationID produces the correct key and value", func() {
		f := deps.CorrelationID("x")
		Expect(f.Key).To(Equal("correlation_id"))
		Expect(f.Value).To(Equal("x"))
	})

	It("DurationMs produces the correct key and value", func() {
		f := deps.DurationMs(42)
		Expect(f.Key).To(Equal("duration_ms"))
		Expect(f.Value).To(Equal(int64(42)))
	})

	It("Attempts produces the correct key and value", func() {
		f := deps.Attempts(3)
		Expect(f.Key).To(Equal("attempts"))
		Expect(f.Value).To(Equal(3))
	})

	It("Err produces the correct key and value", func() {
		someErr := errors.New("test error")
		f := deps.Err(someErr)
		Expect(f.Key).To(Equal("error"))
		Expect(f.Value).To(Equal(someErr))
	})
})

var _ = Describe("LogLevel alignment with zap", func() {
	It("LevelWarn suppresses info-level logs", func() {
		buf := new(bytes.Buffer)
		logger := deps.NewJSONFSMLogger(buf, deps.LevelWarn)
		logger.Info("should be suppressed")

		Expect(buf.String()).To(BeEmpty())
	})

	It("LevelDebug outputs debug-level logs", func() {
		buf := new(bytes.Buffer)
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		logger.Debug("should appear")

		m := parseLine(buf)
		Expect(m).To(HaveKeyWithValue("msg", "should appear"))
		Expect(m).To(HaveKeyWithValue("level", "debug"))
	})
})
