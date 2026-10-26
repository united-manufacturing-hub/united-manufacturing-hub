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
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
)

const unsTopicPrefix = "umh.v1."

var builtinDataContracts = []string{"_raw", "_historian"}

var dataContractVersionSuffix = regexp.MustCompile(`_v\d+$`)

// ValidateTopics reports the topics a write flow at locationPath may not subscribe to,
// given the data contracts declared in config.yaml.
func (c DataflowComponentWriteConfig) ValidateTopics(locationPath string, dataContracts []string) error {
	versioned, unversioned := splitByVersion(dataContracts)
	grammar := newWriteTopicGrammar(locationPath, append(slices.Clone(builtinDataContracts), versioned...))

	rejected := rejectTopics(c.Topics, grammar)
	if len(rejected) == 0 {
		return nil
	}

	return topicError(rejected, grammar, versioned, unversioned)
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

// rejectedTopic is a write flow topic together with the first rule it breaks.
type rejectedTopic struct {
	topic     string
	violation topicViolation
	// syntaxError is why the topic is no regular expression, for topicInvalidRegex.
	syntaxError string
}

func rejectTopics(topics []string, grammar writeTopicGrammar) []rejectedTopic {
	var rejected []rejectedTopic

	for _, topic := range topics {
		if topic == PlaceholderUMHTopicUnset {
			continue
		}

		violation, syntaxError := grammar.check(topic)
		if violation != topicAllowed {
			rejected = append(rejected, rejectedTopic{topic: topic, violation: violation, syntaxError: syntaxError})
		}
	}

	return rejected
}

func topicError(rejected []rejectedTopic, grammar writeTopicGrammar, versioned, unversioned []string) error {
	problems := make([]string, 0, len(rejected)+len(unversioned))
	missingContract := false

	for _, topic := range rejected {
		problems = append(problems, topic.problem(grammar))
		missingContract = missingContract || topic.violation == topicNoDataContract
	}

	if missingContract {
		for _, contract := range unversioned {
			problems = append(problems, fmt.Sprintf(
				"data contract %q has no version; rename it with one, like %s_v1, before a write flow can use it",
				contract, contract))
		}
	}

	return fmt.Errorf("the write flow topics need changes: %s. Example of a valid topic: %s",
		strings.Join(problems, "; "), grammar.exampleTopic(versioned))
}

// topicViolation is the first rule a write flow topic breaks, in the order checked.
type topicViolation int

const (
	topicAllowed topicViolation = iota
	topicInvalidRegex
	topicUnanchored
	topicMatchesNothing
	topicOutsideLocation
	topicNoDataContract
)

// problem describes in one sentence how the topic breaks its rule. The topic is quoted
// as written, so its backslashes read the same as in config.yaml.
func (t rejectedTopic) problem(grammar writeTopicGrammar) string {
	quoted := `"` + t.topic + `"`

	switch t.violation {
	case topicInvalidRegex:
		return fmt.Sprintf("topic %s is not a valid regular expression: %s", quoted, t.syntaxError)
	case topicUnanchored:
		return fmt.Sprintf("topic %s must start with ^, otherwise it also matches topics that only contain it", quoted)
	case topicMatchesNothing:
		return fmt.Sprintf("topic %s cannot match any topic, so the write flow would receive no messages", quoted)
	case topicOutsideLocation:
		if grammar.locationPath == "" {
			return fmt.Sprintf("topic %s can match topics outside the UNS; start it with %s", quoted, grammar.anchoredPrefix())
		}

		return fmt.Sprintf("topic %s can match topics outside this bridge's location %s; start it with %s",
			quoted, grammar.locationPath, grammar.anchoredPrefix())
	case topicNoDataContract:
		return fmt.Sprintf("topic %s must name exactly one data contract after the location, followed by \\.; "+
			"available: %s; list versions explicitly, like _pump_v[1-2]\\.", quoted, strings.Join(grammar.contracts, ", "))
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
	locationPath     string
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
		locationPath:     locationPath,
		locationPrefix:   []rune(prefix),
		contracts:        contracts,
		contractPrefixes: contractPrefixes,
	}
}

// anchoredPrefix is the start every write flow topic of the bridge needs, such as
// `^umh\.v1\.acme\.cologne\.`.
func (g writeTopicGrammar) anchoredPrefix() string {
	return "^" + regexp.QuoteMeta(string(g.locationPrefix))
}

// exampleTopic is a valid topic for the first declared versioned contract, or for
// `_historian` when none is declared.
func (g writeTopicGrammar) exampleTopic(versioned []string) string {
	contract := "_historian"
	if len(versioned) > 0 {
		contract = versioned[0]
	}

	return g.anchoredPrefix() + regexp.QuoteMeta(contract) + `\.`
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

// check returns the first rule pattern breaks, or topicAllowed. For topicInvalidRegex it
// also returns the syntax error.
func (g writeTopicGrammar) check(pattern string) (topicViolation, string) {
	analysis, err := analyzeTopicPattern(pattern, g)
	if err != nil {
		var syntaxErr *syntax.Error
		if errors.As(err, &syntaxErr) {
			return topicInvalidRegex, string(syntaxErr.Code)
		}

		return topicInvalidRegex, err.Error()
	}

	if analysis.MatchesMidTopic {
		return topicUnanchored, ""
	}

	if len(analysis.EndStates) == 0 {
		return topicMatchesNothing, ""
	}

	violations := map[topicViolation]bool{}
	for _, state := range analysis.EndStates {
		violations[state.violation()] = true
	}

	if violations[topicOutsideLocation] {
		return topicOutsideLocation, ""
	}

	if violations[topicNoDataContract] || !namesOneContract(analysis.EndStates) {
		return topicNoDataContract, ""
	}

	return topicAllowed, ""
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
