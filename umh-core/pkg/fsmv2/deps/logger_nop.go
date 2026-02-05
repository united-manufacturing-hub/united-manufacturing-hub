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

package deps

// nopLogger implements FSMLogger but discards all output.
// Use this in unit tests where log output is not needed.
type nopLogger struct{}

// NewNopFSMLogger creates a no-op FSMLogger that discards all log output.
// Use this in unit tests:
//
//	logger := NewNopFSMLogger()
//	worker := NewWorker(logger, ...)
func NewNopFSMLogger() FSMLogger {
	return &nopLogger{}
}

func (l *nopLogger) Debug(_ string, _ ...Field) {}

func (l *nopLogger) Info(_ string, _ ...Field) {}

func (l *nopLogger) SentryWarn(_ Feature, _ string, _ ...Field) {}

func (l *nopLogger) SentryError(_ Feature, _ error, _ string, _ ...Field) {}

func (l *nopLogger) With(_ ...Field) FSMLogger {
	return l
}
