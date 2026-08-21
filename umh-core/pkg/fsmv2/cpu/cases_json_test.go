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

// Cases published as a checked-in JSON file, and the spec that keeps the file
// equal to Cases.
//
// The file exists because software outside this repository shows the same
// customer-visible message strings and holds its own copy of them. One such
// copy has already gone stale, and nobody noticed, because nothing compared
// the two. A function that marshalled Cases in memory would not have caught
// it either: what catches it is a file in the repository, so that rewording a
// message shows up as a diff on a line a reviewer reads in the pull request.
//
// The spec below regenerates the file from Cases and fails when the bytes on
// disk differ. Without it the file is a third copy free to rot alongside the
// other two.

package fsmv2cpu

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
)

// casesJSONPath is the published file, beside cases.go. A reader who changes a
// case sees the regenerated data in the same directory and, in a pull request,
// next to the change that caused it.
const casesJSONPath = "cases.json"

// casesJSONUpdateEnv set to any non-empty value makes the spec below write the
// file instead of only comparing against it.
const casesJSONUpdateEnv = "UPDATE_CPU_CASES_JSON"

// casesJSONRefreshCommand is what to run after changing Cases.
const casesJSONRefreshCommand = "UPDATE_CPU_CASES_JSON=1 go test -tags=test -count=1 ./pkg/fsmv2/cpu/"

// casesJSONRefreshInstruction names the directory to run the command from, and
// goes wherever the command goes. The package path in it is relative, so the
// command works from the module root and nowhere else — including this
// directory, which is where a reader who has just opened the file is standing.
// It ends on the command rather than on punctuation, so a copied line is the
// command and not the command plus a full stop.
const casesJSONRefreshInstruction = "run this from the umh-core module root: " + casesJSONRefreshCommand

// casesJSONTicksNote glosses the one published field a consumer cannot read
// off its own name. Case.Ticks documents the off-by-one against the read
// count, but a consumer sees the JSON and nothing else, so the file has to
// carry the gloss itself.
const casesJSONTicksNote = "ticks is how many one-second clock advances happen before the read this entry answers for; " +
	"one read runs before the first tick, so the answer is the last of ticks+1 reads."

// casesJSONNote heads the file. JSON carries no comments, so this field is the
// only prose a consumer ever sees, and it is where the file says where it came
// from, how to refresh it and what its one unguessable field counts.
const casesJSONNote = "Generated from pkg/fsmv2/cpu/cases.go; do not edit by hand. " +
	casesJSONTicksNote + " To refresh this file, " + casesJSONRefreshInstruction

// publishedCases is the whole file: the note, then the situations in the order
// Cases states them.
type publishedCases struct {
	Note  string          `json:"note"`
	Cases []publishedCase `json:"cases"`
}

// publishedCase is one Case as JSON. Eight of Case's ten fields keep their
// spelling, lower-camelled, so a reader maps those keys straight back to
// cases.go. Two are renamed and are the only two worth memorising: Box is
// published as machine, and PollError as readError.
//
// The nesting is the grouping Case's own documentation already makes. Box
// becomes the machine object, and the five answer fields become answer, which
// readError replaces.
type publishedCase struct {
	Name    string           `json:"name"`
	Why     string           `json:"why"`
	Machine publishedMachine `json:"machine"`
	Ticks   int              `json:"ticks"`

	// Answer is absent exactly when ReadError is present, which is the rule
	// Case.PollError states: a machine whose sample cannot be read produced no
	// verdict, no message and no counts, so there is nothing for the five
	// fields to say.
	Answer    *publishedAnswer `json:"answer,omitempty"`
	ReadError string           `json:"readError,omitempty"`
}

// publishedMachine is fakebox.Condition as JSON.
type publishedMachine struct {
	Cores       int     `json:"cores"`
	QuotaCores  float64 `json:"quotaCores"`
	UsageCores  float64 `json:"usageCores"`
	HostBusy    float64 `json:"hostBusy"`
	Steal       float64 `json:"steal"`
	Throttle    float64 `json:"throttle"`
	Pressure    float64 `json:"pressure"`
	PsiPresent  bool    `json:"psiPresent"`
	Virtualized bool    `json:"virtualized"`
	Affinity    int     `json:"affinity"`

	// Unreadable is written as an empty array rather than null on a machine
	// that can read everything, so a consumer can iterate it without a nil
	// check.
	Unreadable []string `json:"unreadable"`
}

// publishedAnswer is the whole answer for a machine that could be read.
type publishedAnswer struct {
	Verdict string `json:"verdict"`

	// Message is the customer-visible text as one string, headline and
	// Technical Details together. The newlines inside it are JSON string
	// escapes, so a consumer reads back the exact bytes ComposeMessage
	// produced.
	Message string `json:"message"`

	SignalsCapable    int  `json:"signalsCapable"`
	SignalsMeasured   int  `json:"signalsMeasured"`
	RefusingAdmission bool `json:"refusingAdmission"`
}

// renderCasesJSON is the whole file's bytes, built from Cases. It is the single
// producer: the spec compares against it and the refresh writes it, so the two
// cannot disagree about what the file should hold.
func renderCasesJSON() ([]byte, error) {
	published := publishedCases{
		Note:  casesJSONNote,
		Cases: make([]publishedCase, 0, len(Cases)),
	}

	for _, c := range Cases {
		entry := publishedCase{
			Name:      c.Name,
			Why:       c.Why,
			Machine:   publishMachine(c.Box),
			Ticks:     c.Ticks,
			ReadError: c.PollError,
		}

		if c.PollError == "" {
			entry.Answer = &publishedAnswer{
				Verdict:           c.Verdict,
				Message:           c.Message,
				SignalsCapable:    c.SignalsCapable,
				SignalsMeasured:   c.SignalsMeasured,
				RefusingAdmission: c.RefusingAdmission,
			}
		}

		published.Cases = append(published.Cases, entry)
	}

	return encodeCasesJSON(published)
}

func publishMachine(c fakebox.Condition) publishedMachine {
	unreadable := c.Unreadable
	if unreadable == nil {
		unreadable = []string{}
	}

	return publishedMachine{
		Cores:       c.Cores,
		QuotaCores:  c.QuotaCores,
		UsageCores:  c.UsageCores,
		HostBusy:    c.HostBusy,
		Steal:       c.Steal,
		Throttle:    c.Throttle,
		Pressure:    c.Pressure,
		PsiPresent:  c.PsiPresent,
		Virtualized: c.Virtualized,
		Affinity:    c.Affinity,
		Unreadable:  unreadable,
	}
}

// encodeCasesJSON writes one value as the file's bytes.
//
// Two settings decide the bytes. Escaping is off, because the encoder's
// default rewrites <, > and & inside a string as \u003c, \u003e and \u0026,
// which changes the text a consumer reads back for a reason that has nothing
// to do with the message. Indentation is two spaces, so rewording one message
// is a one-line diff rather than a rewrite of the whole file.
//
// The same input then produces the same bytes on any machine: struct fields
// encode in declaration order, nothing here iterates a map, and nothing reads
// a clock, a path or an environment.
func encodeCasesJSON(v any) ([]byte, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// caseFields and conditionFields are the rosters of what the file has to
// carry, written out here for the reason caseNames is written out: a check
// that derives its expectation from the thing it is checking cannot notice a
// field being added. A new field on Case or Condition fails here, which is
// where the reader is told to publish it too, rather than being silently
// absent from the file.
//
// What they do not establish is worth saying, because they sit beside the
// comparison and read as if they covered more than they do. The comparison
// establishes that the file equals what renderCasesJSON produced, and the
// rosters establish that Go has gained no field the roster has not been told
// about. Neither establishes that renderCasesJSON is right. Three changes pass
// once the file is refreshed: dropping a published field, assigning a field
// the wrong source, and adding a field to Case and to the roster here while
// publishing nothing.
//
// That is the structural bound of a file generated from the code it describes,
// and chasing it is not worth the machinery. Each of those three rewrites all
// thirteen entries, so each arrives as a diff across the whole file in the pull
// request. Being readable there is what the file is for.
var caseFields = []string{
	"Name", "Why", "Box", "Ticks", "Verdict", "Message",
	"SignalsCapable", "SignalsMeasured", "RefusingAdmission", "PollError",
}

var conditionFields = []string{
	"Cores", "QuotaCores", "UsageCores", "HostBusy", "Steal", "Throttle",
	"Pressure", "PsiPresent", "Virtualized", "Affinity", "Unreadable",
}

func fieldNames(v any) []string {
	t := reflect.TypeOf(v)
	names := make([]string, 0, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		names = append(names, t.Field(i).Name)
	}

	return names
}

var _ = Describe("the situations published as a file", func() {
	It("holds what Cases holds", func() {
		want, err := renderCasesJSON()
		Expect(err).NotTo(HaveOccurred())

		if os.Getenv(casesJSONUpdateEnv) != "" {
			Expect(os.WriteFile(casesJSONPath, want, 0o644)).To(Succeed())
		}

		got, err := os.ReadFile(casesJSONPath)
		Expect(err).NotTo(HaveOccurred(),
			casesJSONPath+" is missing. To write it, "+casesJSONRefreshInstruction)

		Expect(string(got)).To(Equal(string(want)),
			casesJSONPath+" no longer matches Cases. To refresh it, "+casesJSONRefreshInstruction)
	})

	It("produces the same bytes every time it is built", func() {
		first, err := renderCasesJSON()
		Expect(err).NotTo(HaveOccurred())

		second, err := renderCasesJSON()
		Expect(err).NotTo(HaveOccurred())

		Expect(second).To(Equal(first))
	})

	It("leaves <, > and & in a message alone", func() {
		// Nothing in Cases carries one of these today, so the encoder's
		// escaping setting would be unguarded without a value that does. An
		// encoder left on its default rewrites all three and the check below
		// fails.
		got, err := encodeCasesJSON(publishedAnswer{Message: "load <5 & rising >"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(got)).To(ContainSubstring("load <5 & rising >"))
	})

	It("carries every field a Case has", func() {
		Expect(fieldNames(Case{})).To(Equal(caseFields),
			"a field added to Case must be published in cases.json too")
	})

	It("carries every field a machine condition has", func() {
		Expect(fieldNames(fakebox.Condition{})).To(Equal(conditionFields),
			"a field added to fakebox.Condition must be published in cases.json too")
	})

	Describe("the messages it publishes", func() {
		var file publishedCases

		BeforeEach(func() {
			raw, err := os.ReadFile(casesJSONPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(json.Unmarshal(raw, &file)).To(Succeed())
		})

		It("reads back the exact bytes every case states", func() {
			Expect(file.Cases).To(HaveLen(len(Cases)))

			for i, c := range Cases {
				published := file.Cases[i]
				Expect(published.Name).To(Equal(c.Name))

				if c.PollError != "" {
					Expect(published.Answer).To(BeNil())
					Expect(published.ReadError).To(Equal(c.PollError))

					continue
				}

				Expect(published.Answer).NotTo(BeNil())
				Expect(published.Answer.Message).To(Equal(c.Message))
			}
		})

		It("keeps a multi-line message in one string field", func() {
			// throttled is the longest of them: a headline, a blank line
			// between two cause paragraphs, and three real newlines in all.
			// Read back it is one string, and on disk those newlines are
			// escapes rather than line breaks that would split the field
			// across lines.
			var throttled string
			for _, c := range file.Cases {
				if c.Name == "throttled" {
					Expect(c.Answer).NotTo(BeNil())
					throttled = c.Answer.Message
				}
			}

			Expect(throttled).NotTo(BeEmpty(), "throttled must be published")
			Expect(throttled).To(ContainSubstring("\n\n"))

			raw, err := os.ReadFile(casesJSONPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).To(ContainSubstring(`\nTechnical Details:`))
		})
	})
})
