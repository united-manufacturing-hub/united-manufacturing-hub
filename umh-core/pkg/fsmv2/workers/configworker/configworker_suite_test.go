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

package configworker_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestConfigWorker bootstraps the Ginkgo suite for this package. The package's
// own tests are plain testing.T functions, but the suite-wide unit-test command
// passes -ginkgo.* flags to every package binary; without a Ginkgo suite the
// binary rejects those unknown flags and exits before any test runs.
func TestConfigWorker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ConfigWorker Suite")
}
