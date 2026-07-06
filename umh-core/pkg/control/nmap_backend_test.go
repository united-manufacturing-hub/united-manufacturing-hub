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

package control

// RED-phase (TDD) spec for the loop.go global-nmap-manager gate.
//
// ASSUMED SEAM (drives GREEN — implement EXACTLY this in pkg/control):
//
//	func globalNmapManagerEnabled() bool {
//	    return env.GetAsString("NMAP_BACKEND", false, "fsmv1") != constants.NmapBackendFSMv2
//	}
//
// NewControlLoop appends the global fsmv1 nmap.NewNmapManager(...) only when
// this helper reports true. When NMAP_BACKEND=fsmv2 the connection service owns
// an embedded fsmv2 nmap manager, so the global fsmv1 manager MUST be skipped to
// avoid double-managing config.Internal.Nmap.
//
// This file fails to compile until globalNmapManagerEnabled and
// constants.NmapBackendFSMv2 exist.

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

var _ = Describe("globalNmapManagerEnabled", func() {
	AfterEach(func() {
		_ = os.Unsetenv("NMAP_BACKEND")
	})

	It("is disabled when NMAP_BACKEND=fsmv2 (fsmv2 manager owns nmap)", func() {
		_ = os.Setenv("NMAP_BACKEND", constants.NmapBackendFSMv2)

		defer func() { _ = os.Unsetenv("NMAP_BACKEND") }()

		Expect(globalNmapManagerEnabled()).To(BeFalse(),
			"the global fsmv1 nmap manager must be skipped when the fsmv2 backend is on")
	})

	It("is enabled when NMAP_BACKEND is unset (FF-off default)", func() {
		_ = os.Unsetenv("NMAP_BACKEND")

		Expect(globalNmapManagerEnabled()).To(BeTrue(),
			"the global fsmv1 nmap manager must be appended by default")
	})

	It("is enabled for any non-fsmv2 value", func() {
		_ = os.Setenv("NMAP_BACKEND", "fsmv1")

		defer func() { _ = os.Unsetenv("NMAP_BACKEND") }()

		Expect(globalNmapManagerEnabled()).To(BeTrue())
	})
})
