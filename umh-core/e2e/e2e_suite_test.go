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

package e2e_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

// Global logger for e2e tests
var e2eLogger *zap.SugaredLogger

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UMH Core E2E Test Suite")
}

var _ = BeforeSuite(func() {
	// Set up global logger for e2e tests
	logger, _ := zap.NewDevelopment()
	e2eLogger = logger.Sugar()
})

var _ = AfterSuite(func() {
	// Clean up global logger
	if e2eLogger != nil {
		_ = e2eLogger.Sync()
	}
})
