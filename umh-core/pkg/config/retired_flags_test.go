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

package config

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Retired FSMv2 flags must keep parsing. Nothing reads them, which makes them
// look like dead code, but ParseConfig runs with allowUnknownFields=false so
// KnownFields is on: a key with no matching struct field fails the whole
// document. Deleting one stops an instance from loading a config.yaml that
// still carries it, at startup and on every background refresh, and the
// operator sees a config error rather than an ignored setting.
//
// The negative case is asserted alongside on purpose. Without it these specs
// would pass even if strict parsing were switched off, which is the state that
// makes the positive case meaningless.
var _ = Describe("retired FSMv2 config flags", func() {
	parse := func(yaml string) error {
		_, err := ParseConfig([]byte(yaml), context.Background(), false)

		return err
	}

	// The paths differ: three sit directly under agent, while useFSMv2Transport
	// sits under agent.communicator, because CommunicatorConfig is embedded as
	// yaml:"communicator". Spelling each one out keeps that difference visible,
	// since a document written at the wrong path fails rather than being ignored.
	DescribeTable("a config.yaml carrying a retired flag still loads",
		func(yaml string) {
			Expect(parse(yaml)).To(Succeed(),
				"deleting this field is a breaking change: instances with the key on disk stop loading their config")
		},
		Entry("agent.enableFSMv2", "agent:\n  enableFSMv2: true\n"),
		Entry("agent.useFSMv2MemoryCleanup", "agent:\n  useFSMv2MemoryCleanup: true\n"),
		Entry("agent.useFSMv2ProtocolConverter", "agent:\n  useFSMv2ProtocolConverter: true\n"),
		Entry("agent.communicator.useFSMv2Transport", "agent:\n  communicator:\n    useFSMv2Transport: true\n"),
	)

	It("rejects a key with no struct field, so the specs above are not vacuous", func() {
		Expect(parse("agent:\n  totallyBogusKey: true\n")).To(HaveOccurred(),
			"strict parsing is off, so the retired-flag specs prove nothing")
	})
})
