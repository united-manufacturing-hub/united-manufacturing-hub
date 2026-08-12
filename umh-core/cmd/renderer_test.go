// Copyright 2026 UMH Systems GmbH
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

package main

import (
	"bytes"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

var _ = Describe("renderSupervisorChildrenYAML", func() {
	// renderSupervisorChildrenYAML is the pure core of buildFSMv2Supervisor's
	// child selection. The design decision that matters is that choosing the
	// children moves out of the effectful builder, so the central invariant --
	// persistence always, communicator iff credentials -- is provable without a
	// supervisor, a store or a goroutine.

	type decodedChild struct {
		Name       string `yaml:"name"`
		WorkerType string `yaml:"workerType"`
		UserSpec   struct {
			Config string `yaml:"config"`
		} `yaml:"userSpec"`
	}
	type decodedChildren struct {
		Children []decodedChild `yaml:"children"`
	}
	type decodedUserConfig struct {
		RelayURL     string `yaml:"relayURL"`
		InstanceUUID string `yaml:"instanceUUID"`
		AuthToken    string `yaml:"authToken"`
	}

	childNames := func(doc string) []string {
		var parsed decodedChildren
		Expect(yaml.Unmarshal([]byte(doc), &parsed)).To(Succeed())
		out := make([]string, 0, len(parsed.Children))
		for _, c := range parsed.Children {
			out = append(out, c.Name)
		}

		return out
	}

	// render fails the spec on a render error rather than asserting against an
	// empty document: an empty document means a supervisor with no children at
	// all, which no assertion below would distinguish from a correct render.
	render := func(cfg config.AgentConfig, instanceUUID string) string {
		doc, err := renderSupervisorChildrenYAML(cfg, instanceUUID)
		Expect(err).To(Succeed(), "rendering the supervisor children must not fail")

		return doc
	}

	It("always renders persistence and communicator only with credentials, round-tripping byte-for-byte", func() {
		// Lockstep half of Property 8: the persistence child and its deps
		// registration are both unconditional. The pure core cannot register
		// deps, so that half is asserted at the source: no read of
		// UseFSMv2MemoryCleanup may remain in cmd/main.go, or persistence could
		// spawn without its deps.
		src, err := os.ReadFile("main.go")
		Expect(err).To(Succeed())
		Expect(bytes.Contains(src, []byte("UseFSMv2MemoryCleanup"))).To(BeFalse(),
			"persistence child + deps registration must become unconditional in lockstep; "+
				"the memory-cleanup gate is still read in cmd/main.go")

		// Zero AgentConfig: no credentials, no memory-cleanup. Persistence must
		// appear regardless; communicator must not (there is no runtime to wire it to).
		Expect(childNames(render(config.AgentConfig{}, "uuid-0"))).
			To(ContainElement("persistence"))
		Expect(childNames(render(config.AgentConfig{}, "uuid-0"))).
			NotTo(ContainElement("communicator"))

		// Credentials present: communicator appears, and its nested userSpec
		// config must decode back to the exact bytes passed in -- including the
		// hostile token (E14/E15), which disqualifies fmt.Sprintf inline
		// building and forces yaml.Marshal.
		const (
			relayURL        = `http://fsmv2.invalid:9999`
			authToken       = "token\"with\\backslash\nand-newline"
			placeholderUUID = "placeholder-1234"
		)
		credentialed := config.AgentConfig{CommunicatorConfig: config.CommunicatorConfig{
			APIURL:    relayURL,
			AuthToken: authToken,
		}}
		doc := render(credentialed, placeholderUUID)
		Expect(childNames(doc)).To(ContainElement("persistence"))
		Expect(childNames(doc)).To(ContainElement("communicator"))

		var parsed decodedChildren
		Expect(yaml.Unmarshal([]byte(doc), &parsed)).To(Succeed())
		var comm *decodedChild
		for i := range parsed.Children {
			if parsed.Children[i].Name == "communicator" {
				comm = &parsed.Children[i]

				break
			}
		}
		Expect(comm).NotTo(BeNil(), "communicator child must be present in the emitted YAML")
		var userCfg decodedUserConfig
		Expect(yaml.Unmarshal([]byte(comm.UserSpec.Config), &userCfg)).To(Succeed())
		Expect(userCfg.RelayURL).To(Equal(relayURL))
		Expect(userCfg.AuthToken).To(Equal(authToken))
		Expect(userCfg.InstanceUUID).To(Equal(placeholderUUID))

		// Partial credentials (only API URL) are treated as no credentials
		// (E2): the existing && contract must be preserved in the renderer.
		partial := config.AgentConfig{CommunicatorConfig: config.CommunicatorConfig{APIURL: relayURL}}
		Expect(childNames(render(partial, "uuid-1"))).
			NotTo(ContainElement("communicator"))
	})
})
