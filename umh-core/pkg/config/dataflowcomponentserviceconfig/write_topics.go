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

// Each write flow topic is a Go regular expression that the benthos-umh `uns` input
// matches unanchored against the full UNS topic name. The rules apply to every topic the
// expression can match, not to how it reads:
//
//   - It starts with `^`. Without it, the expression also matches any topic that merely
//     contains it.
//   - It matches only topics strictly beneath the bridge location, so a write flow
//     consumes only its own branch of the UNS tree. An unescaped `.` matches any
//     character, so the dots between levels must be escaped.
//   - The first `_`-prefixed segment beneath the location is `_raw`, `_historian` or a
//     versioned name (`_<name>_v<N>`) from `dataContracts:`, followed by a dot. All
//     matched topics name versions of one contract: `_pump_v[1-2]\.` is valid when
//     `_pump_v1` and `_pump_v2` are declared, while `_pump_v\d+\.` reaches undeclared
//     versions and a wildcard in place of the contract reaches every contract.
//
// Anything may follow the contract segment. Topics must already be rendered, with every
// variable rendered as an escaped literal; an unrendered `{{ }}` action is read as literal
// text and reported.

package dataflowcomponentserviceconfig

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/topicpattern"
)

const unsTopicPrefix = "umh.v1."

var builtinDataContracts = []string{"_raw", "_historian"}

var dataContractVersionSuffix = regexp.MustCompile(`_v\d+$`)

// ValidateTopics reports the topics a write flow at locationPath may not subscribe to.
func (c DataflowComponentWriteConfig) ValidateTopics(locationPath string, dataContracts []string) error {
	versioned, unversioned := splitByVersion(dataContracts)
	grammar := newWriteTopicGrammar(locationPath, append(slices.Clone(builtinDataContracts), versioned...))

	rejected := classifyTopics(c.Topics, grammar)
	if len(rejected) == 0 {
		return nil
	}

	return topicError(rejected, grammar, unversioned)
}

// splitByVersion separates declared data contracts with a `_v<N>` suffix from those
// without, which a write flow cannot use.
func splitByVersion(dataContracts []string) (versioned, unversioned []string) {
	for _, contract := range dataContracts {
		if dataContractVersionSuffix.MatchString(contract) {
			versioned = append(versioned, contract)
		} else {
			unversioned = append(unversioned, contract)
		}
	}

	return versioned, unversioned
}

func classifyTopics(topics []string, grammar writeTopicGrammar) map[topicViolation][]string {
	rejected := map[topicViolation][]string{}

	for _, topic := range topics {
		if topic == PlaceholderUMHTopicUnset {
			continue
		}

		violation := grammar.check(topic)
		if violation != topicAllowed {
			rejected[violation] = append(rejected[violation], topic)
		}
	}

	return rejected
}

func topicError(rejected map[topicViolation][]string, grammar writeTopicGrammar, unversioned []string) error {
	var problems []string

	for _, violation := range reportedViolations {
		if topics := rejected[violation]; len(topics) > 0 {
			problems = append(problems, violation.problem(grammar)+": "+strings.Join(topics, ", "))
		}
	}

	if len(rejected[topicNoDataContract]) > 0 && len(unversioned) > 0 {
		problems = append(problems, fmt.Sprintf(
			"data contracts %s are declared without a version and need one such as _v1 before a write flow can use them",
			strings.Join(unversioned, ", ")))
	}

	return fmt.Errorf("write flow topics %s", strings.Join(problems, "; "))
}

// topicViolation is the reason a write flow topic is rejected.
type topicViolation int

const (
	topicAllowed topicViolation = iota
	topicInvalidRegex
	topicUnanchored
	topicOutsideLocation
	topicNoDataContract
)

// reportedViolations is the order in which violations appear in the error.
var reportedViolations = []topicViolation{topicInvalidRegex, topicUnanchored, topicOutsideLocation, topicNoDataContract}

func (v topicViolation) problem(grammar writeTopicGrammar) string {
	switch v {
	case topicInvalidRegex:
		return "must be valid regular expressions"
	case topicUnanchored:
		return "must start with ^"
	case topicOutsideLocation:
		return "must be beneath the bridge location, starting with ^" + regexp.QuoteMeta(string(grammar.locationPrefix))
	case topicNoDataContract:
		return fmt.Sprintf("must each name one existing data contract followed by \\., listing versions explicitly (%s)",
			strings.Join(grammar.contracts, ", "))
	default:
		return ""
	}
}

// writeTopicPhase is the part of a UNS topic the grammar is reading.
type writeTopicPhase int

const (
	// phaseLocationPrefix reads `umh.v1.` and the bridge location, followed by a dot.
	phaseLocationPrefix writeTopicPhase = iota
	phaseLevelStart
	phaseLevel
	// phaseContract reads a segment starting with `_`, the data contract.
	phaseContract
	// phaseAccepted, phaseOutsideLocation and phaseNoContract are final: no later rune
	// changes them.
	phaseAccepted
	phaseOutsideLocation
	phaseNoContract
)

type writeTopicState struct {
	phase writeTopicPhase
	// prefixRead counts the runes of the location prefix read, in phaseLocationPrefix.
	prefixRead int
	// contract is the contract name read so far in phaseContract, and the accepted
	// contract in phaseAccepted.
	contract string
}

// writeTopicGrammar accepts topics beneath a bridge location whose first `_` segment is
// one of a fixed list of data contracts.
type writeTopicGrammar struct {
	locationPrefix   []rune
	contracts        []string
	contractPrefixes map[string]bool
}

func newWriteTopicGrammar(locationPath string, contracts []string) writeTopicGrammar {
	prefix := unsTopicPrefix
	if locationPath != "" {
		prefix += locationPath + "."
	}

	contractPrefixes := map[string]bool{}
	for _, contract := range contracts {
		for end := 1; end <= len(contract); end++ {
			contractPrefixes[contract[:end]] = true
		}
	}

	return writeTopicGrammar{
		locationPrefix:   []rune(prefix),
		contracts:        contracts,
		contractPrefixes: contractPrefixes,
	}
}

func (g writeTopicGrammar) Start() writeTopicState {
	return writeTopicState{phase: phaseLocationPrefix}
}

func (g writeTopicGrammar) Significant() []rune {
	significant := []rune{'.', '_'}
	significant = append(significant, g.locationPrefix...)
	significant = append(significant, []rune(strings.Join(g.contracts, ""))...)

	return significant
}

func (g writeTopicGrammar) Step(state writeTopicState, r rune) writeTopicState {
	switch state.phase {
	case phaseLocationPrefix:
		return g.stepLocationPrefix(state, r)
	case phaseLevelStart:
		return stepLevelStart(r)
	case phaseLevel:
		return stepLevel(state, r)
	case phaseContract:
		return g.stepContract(state, r)
	default:
		return state
	}
}

func (g writeTopicGrammar) stepLocationPrefix(state writeTopicState, r rune) writeTopicState {
	if r != g.locationPrefix[state.prefixRead] {
		return writeTopicState{phase: phaseOutsideLocation}
	}

	if state.prefixRead+1 == len(g.locationPrefix) {
		return writeTopicState{phase: phaseLevelStart}
	}

	return writeTopicState{phase: phaseLocationPrefix, prefixRead: state.prefixRead + 1}
}

func stepLevelStart(r rune) writeTopicState {
	switch r {
	case '_':
		return writeTopicState{phase: phaseContract, contract: "_"}
	case '.':
		return writeTopicState{phase: phaseNoContract}
	default:
		return writeTopicState{phase: phaseLevel}
	}
}

func stepLevel(state writeTopicState, r rune) writeTopicState {
	if r == '.' {
		return writeTopicState{phase: phaseLevelStart}
	}

	return state
}

func (g writeTopicGrammar) stepContract(state writeTopicState, r rune) writeTopicState {
	if r == '.' && slices.Contains(g.contracts, state.contract) {
		return writeTopicState{phase: phaseAccepted, contract: state.contract}
	}

	next := state.contract + string(r)
	if !g.contractPrefixes[next] {
		return writeTopicState{phase: phaseNoContract}
	}

	return writeTopicState{phase: phaseContract, contract: next}
}

func (s writeTopicState) violation() topicViolation {
	switch s.phase {
	case phaseAccepted:
		return topicAllowed
	case phaseLocationPrefix, phaseOutsideLocation:
		return topicOutsideLocation
	default:
		return topicNoDataContract
	}
}

// check returns why a write flow may not subscribe to pattern, or topicAllowed.
func (g writeTopicGrammar) check(pattern string) topicViolation {
	analysis, err := topicpattern.Analyze(pattern, g)
	if err != nil {
		return topicInvalidRegex
	}

	if analysis.MatchesMidTopic {
		return topicUnanchored
	}

	violations := map[topicViolation]bool{}
	for _, state := range analysis.EndStates {
		violations[state.violation()] = true
	}

	if violations[topicOutsideLocation] {
		return topicOutsideLocation
	}

	if violations[topicNoDataContract] || !namesOneContract(analysis.EndStates) {
		return topicNoDataContract
	}

	return topicAllowed
}

// namesOneContract reports whether every accepted state names a version of the same
// data contract, such as _pump_v1 and _pump_v2.
func namesOneContract(states []writeTopicState) bool {
	bases := map[string]bool{}
	for _, state := range states {
		bases[dataContractVersionSuffix.ReplaceAllString(state.contract, "")] = true
	}

	return len(bases) <= 1
}
