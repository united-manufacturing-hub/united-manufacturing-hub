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

package diagnosis

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Specs run with the cooldown after a release, whatever the machine sets;
// engine_refire_test.go turns USE_FSMV2_IMMEDIATE_REFIRE on where it needs it.
var _ = BeforeEach(func() {
	GinkgoT().Setenv(immediateRefireFlag, "")
})

func TestDiagnosis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Diagnosis Suite")
}
