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

package examples_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

// caseHeader opens every rendered block, and caseHeaderEnd closes it. They are
// written out here rather than read from the renderer, so that changing the
// marker on one side and not the other fails: the spec's job is to hold the
// rendered text still, and a marker it imported from the renderer could not
// tell the two apart.
const (
	caseHeader    = "=== situation: "
	caseHeaderEnd = " ===\n"
)

// blocksByCase cuts the rendered page into one body per case, keyed by the
// name in the block's own header.
//
// Every check below reads one body rather than the whole page. Checking the
// page cannot tell a block carrying its own answer from a block carrying its
// neighbour's: rotating the customer messages among the cases leaves every
// string still present somewhere, and a page-wide search passes on a page
// where every situation is captioned with the wrong text.
//
// The returned body starts after the case's name, so a check for a name can
// never be satisfied by the header the name came from.
func blocksByCase(output string) map[string]string {
	blocks := map[string]string{}

	// Element 0 is the preamble, which precedes the first header.
	for _, piece := range strings.Split(output, caseHeader)[1:] {
		end := strings.Index(piece, caseHeaderEnd)
		Expect(end).To(BeNumerically(">", 0),
			"a block header opened but never closed, so the page cannot be cut into blocks")

		name := piece[:end]
		Expect(blocks).NotTo(HaveKey(name), "case "+name+" is rendered more than once")
		blocks[name] = piece[end:]
	}

	return blocks
}

var _ = Describe("CPUHealthScenario", func() {
	It("registers the cpuhealth scenario in the registry for CLI access", func() {
		scenario, exists := examples.Registry["cpuhealth"]
		Expect(exists).To(BeTrue())
		Expect(scenario.Name).To(Equal("cpuhealth"))
		Expect(scenario.YAMLConfig).To(BeEmpty(),
			"the scenario must build the worker directly with a fake machine's sampler, not a YAML-spawned one")
	})

	Describe("what the command line prints", func() {
		var output string

		BeforeEach(func() {
			// Everything below walks fsmv2cpu.Cases and checks the rendering
			// against what it finds. An empty set would leave every one of
			// those walks iterating nothing and passing, so the set being
			// non-empty is the precondition they all rest on.
			Expect(fsmv2cpu.Cases).NotTo(BeEmpty())

			result, err := examples.Registry["cpuhealth"].CustomRunner(context.Background(), examples.RunConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Eventually(result.Done).Should(BeClosed())
			Expect(result.ShutdownClean).To(BeTrue(),
				"the fake-machine scenario has no supervisor, so a clean shutdown is the only honest value")

			output = result.Output
			Expect(output).NotTo(BeEmpty(),
				"the scenario returns what it printed, and everything below reads it")
		})

		It("renders one block per case, so a whole set cannot render as one block", func() {
			// Counted from the data rather than written down. A spec carrying
			// its own number would pass on a set of that size and no other,
			// and would have to be edited every time a case is added.
			Expect(strings.Count(output, caseHeader)).To(Equal(len(fsmv2cpu.Cases)),
				"a renderer that stops after the first case, or repeats one, must fail here")
		})

		It("heads every case with its name, in the order the set states", func() {
			previous := -1
			for _, c := range fsmv2cpu.Cases {
				header := caseHeader + c.Name + " ==="
				at := strings.Index(output, header)
				Expect(at).To(BeNumerically(">", previous),
					"case "+c.Name+" is missing, or is rendered out of the roster order")
				previous = at
			}
		})

		It("says inside each case's own block what that case exists to show", func() {
			blocks := blocksByCase(output)
			for _, c := range fsmv2cpu.Cases {
				Expect(blocks).To(HaveKey(c.Name))
				Expect(blocks[c.Name]).To(ContainSubstring(c.Why),
					"case "+c.Name+" renders without the line saying why it is here")
			}
		})

		It("carries each case's verdict, counts and warm-up state in that case's own block", func() {
			blocks := blocksByCase(output)
			for _, c := range fsmv2cpu.Cases {
				Expect(blocks).To(HaveKey(c.Name))
				block := blocks[c.Name]

				// A read that failed produced no verdict and no counts, which
				// is the rule cases.go states about PollError. Printing any of
				// them would be inventing an answer, so the block must not
				// carry the labels at all.
				if c.PollError != "" {
					Expect(block).NotTo(ContainSubstring("verdict    "),
						"case "+c.Name+" could not be read, so it has no verdict to print")
					Expect(block).NotTo(ContainSubstring("signals    "),
						"case "+c.Name+" could not be read, so it has no counts to print")

					continue
				}

				Expect(block).To(ContainSubstring("verdict    " + c.Verdict + "\n"))
				Expect(block).To(ContainSubstring(
					fmt.Sprintf("signals    %d capable, %d measured\n", c.SignalsCapable, c.SignalsMeasured)))
				Expect(block).To(ContainSubstring("warm-up    " + warmUpText(c.RefusingAdmission) + "\n"))
			}
		})

		It("carries each case's whole customer message, or its read error, in that case's own block", func() {
			blocks := blocksByCase(output)
			for _, c := range fsmv2cpu.Cases {
				Expect(blocks).To(HaveKey(c.Name))

				// A case whose read fails has no message to print. Its answer
				// is the error, so that is what the block has to carry.
				expected := c.Message
				if c.PollError != "" {
					expected = c.PollError
				}

				Expect(blocks[c.Name]).To(ContainSubstring(expected),
					"case "+c.Name+" carries an answer that is not its own")
			}
		})
	})
})

// warmUpText is the wording the page must carry for a RefusingAdmission bit.
// It is written out here rather than imported so that a change to either
// wording fails, which is the whole point of a spec over rendered text.
func warmUpText(refusing bool) string {
	if refusing {
		return "refusing new work until a capable signal first measures"
	}

	return "not refusing: this worker's own start-up adds no block"
}
