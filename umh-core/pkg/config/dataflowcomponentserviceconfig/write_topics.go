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

package dataflowcomponentserviceconfig

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"unicode"
)

// UNSTopicPrefix is the fixed prefix of every UNS topic. The segments after it are the
// location path, followed by the data contract and any virtual path.
const UNSTopicPrefix = "umh.v1."

// BuiltinDataContracts are the data contracts that exist without being declared in the
// `dataContracts:` section of config.yaml: `_raw` carries no validation rules and
// `_historian` is the built-in timeseries contract. Both are unversioned, so the UNS
// output bypasses schema validation for them.
var BuiltinDataContracts = []string{"_raw", "_historian"}

// dataContractVersionSuffix matches the `_v<N>` version suffix of a data contract name.
var dataContractVersionSuffix = regexp.MustCompile(`_v\d+$`)

// ValidateTopics reports the topics a write flow may not subscribe to.
//
// Each topic is a Go regular expression that the benthos-umh `uns` input matches unanchored
// against the full UNS topic name. The rules apply to every topic the expression can match,
// not to how it reads:
//
//   - It starts with `^`. Without it, the expression also matches any topic that merely
//     contains it.
//   - It matches only topics strictly beneath locationPath, so a write flow consumes only
//     its own branch of the UNS tree. An unescaped `.` matches any character, so the dots
//     between levels must be escaped.
//   - The first `_`-prefixed segment beneath locationPath is a data contract out of
//     BuiltinDataContracts and the versioned names (`_<name>_v<N>`) in dataContracts,
//     followed by a dot. All matched topics name versions of one contract: `_pump_v[1-2]\.`
//     is valid when `_pump_v1` and `_pump_v2` are declared, while `_pump_v\d+\.` reaches
//     undeclared versions and a wildcard in place of the contract reaches every contract.
//
// Anything may follow the contract segment.
//
// Topics must already be rendered, with every variable rendered as an escaped literal; an
// unrendered `{{ }}` action is read as literal text and reported.
func (c DataflowComponentWriteConfig) ValidateTopics(locationPath string, dataContracts []string) error {
	contracts := slices.Clone(BuiltinDataContracts)

	var unversioned []string

	for _, contract := range dataContracts {
		if dataContractVersionSuffix.MatchString(contract) {
			contracts = append(contracts, contract)
		} else {
			unversioned = append(unversioned, contract)
		}
	}

	prefix := UNSTopicPrefix
	if locationPath != "" {
		prefix += locationPath + "."
	}

	grammar := newTopicGrammar(prefix, contracts)
	rejected := map[topicViolation][]string{}

	for _, topic := range c.Topics {
		if topic == PlaceholderUMHTopicUnset {
			continue
		}

		if violation := grammar.check(topic); violation != topicAllowed {
			rejected[violation] = append(rejected[violation], topic)
		}
	}

	var problems []string

	for _, group := range []struct {
		problem   string
		violation topicViolation
	}{
		{violation: topicInvalidRegex, problem: "must be valid regular expressions"},
		{violation: topicUnanchored, problem: "must start with ^"},
		{violation: topicOutsideLocation, problem: "must be beneath the bridge location, starting with ^" + regexp.QuoteMeta(prefix)},
		{violation: topicNoDataContract, problem: fmt.Sprintf(
			"must each name one existing data contract followed by \\., listing versions explicitly (%s)",
			strings.Join(contracts, ", "))},
	} {
		if topics := rejected[group.violation]; len(topics) > 0 {
			problems = append(problems, group.problem+": "+strings.Join(topics, ", "))
		}
	}

	if len(problems) == 0 {
		return nil
	}

	if len(rejected[topicNoDataContract]) > 0 && len(unversioned) > 0 {
		problems = append(problems, fmt.Sprintf(
			"data contracts %s are declared without a version and need one such as _v1 before a write flow can use them",
			strings.Join(unversioned, ", ")))
	}

	return fmt.Errorf("write flow topics %s", strings.Join(problems, "; "))
}

// topicViolation is the reason a write flow topic is rejected, ordered from most to least
// fundamental: a topic is reported under the first one it violates.
type topicViolation int

const (
	topicAllowed topicViolation = iota
	topicInvalidRegex
	topicUnanchored
	topicOutsideLocation
	topicNoDataContract
)

// topicSection is the part of a UNS topic the runes read so far end in.
type topicSection int

const (
	// sectionUnanchored is a match that began past the first rune of the topic.
	sectionUnanchored topicSection = iota
	// sectionLocation is inside `umh.v1.` and the bridge location.
	sectionLocation
	// sectionLevelStart is at the start of a segment beneath the bridge location.
	sectionLevelStart
	// sectionLevel is inside a location level beneath the bridge location.
	sectionLevel
	// sectionContract is inside the data contract segment.
	sectionContract
	// sectionVirtualPath is past a valid data contract segment and its dot.
	sectionVirtualPath
	// sectionOutside has left the bridge location.
	sectionOutside
	// sectionInvalidContract has a contract segment that is no existing contract.
	sectionInvalidContract
)

// topicPosition is a state of the topic grammar.
type topicPosition struct {
	// contract is the contract segment read so far, in sectionContract and
	// sectionVirtualPath.
	contract string
	section  topicSection
	// read counts the runes of the location prefix read, in sectionLocation.
	read int
}

// violation reports why a pattern matching up to p is rejected, given that any topic
// continues from there.
func (p topicPosition) violation() topicViolation {
	switch p.section {
	case sectionUnanchored:
		return topicUnanchored
	case sectionLocation, sectionOutside:
		return topicOutsideLocation
	case sectionVirtualPath:
		return topicAllowed
	default:
		return topicNoDataContract
	}
}

// topicGrammar is an automaton over the runes of a UNS topic that tracks whether the topic
// is one a write flow may consume.
type topicGrammar struct {
	// significant holds every rune the grammar tells apart from the others. All remaining
	// runes take the same transitions, so one of them stands in for all.
	significant map[rune]bool
	prefix      []rune
	contracts   []string
}

func newTopicGrammar(prefix string, contracts []string) topicGrammar {
	g := topicGrammar{
		prefix:      []rune(prefix),
		contracts:   contracts,
		significant: map[rune]bool{'.': true, '_': true},
	}

	for _, r := range prefix + strings.Join(contracts, "") {
		g.significant[r] = true
	}

	return g
}

func (g topicGrammar) step(p topicPosition, r rune) topicPosition {
	switch p.section {
	case sectionLocation:
		if r != g.prefix[p.read] {
			return topicPosition{section: sectionOutside}
		}

		if p.read+1 == len(g.prefix) {
			return topicPosition{section: sectionLevelStart}
		}

		return topicPosition{section: sectionLocation, read: p.read + 1}
	case sectionLevelStart:
		switch r {
		case '_':
			return topicPosition{section: sectionContract, contract: "_"}
		case '.':
			return topicPosition{section: sectionInvalidContract}
		default:
			return topicPosition{section: sectionLevel}
		}
	case sectionLevel:
		if r == '.' {
			return topicPosition{section: sectionLevelStart}
		}

		return p
	case sectionContract:
		if r == '.' {
			if slices.Contains(g.contracts, p.contract) {
				return topicPosition{section: sectionVirtualPath, contract: p.contract}
			}

			return topicPosition{section: sectionInvalidContract}
		}

		next := p.contract + string(r)
		if slices.ContainsFunc(g.contracts, func(c string) bool { return strings.HasPrefix(c, next) }) {
			return topicPosition{section: sectionContract, contract: next}
		}

		return topicPosition{section: sectionInvalidContract}
	default:
		return p
	}
}

// check reports whether pattern matches only topics the grammar accepts. It walks the
// compiled pattern and the grammar in lockstep, feeding each rune instruction every
// significant rune it accepts plus one insignificant stand-in, so every topic the pattern
// can match is covered. Topics never contain a newline, so none is fed.
func (g topicGrammar) check(pattern string) topicViolation {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return topicInvalidRegex
	}

	prog, err := syntax.Compile(re.Simplify())
	if err != nil {
		return topicInvalidRegex
	}

	type thread struct {
		at        topicPosition
		pc        uint32
		textStart bool
		textEnd   bool
	}

	// The second thread is a match beginning past the first rune, where `^` cannot hold.
	pending := []thread{
		{pc: uint32(prog.Start), at: topicPosition{section: sectionLocation}, textStart: true},
		{pc: uint32(prog.Start), at: topicPosition{section: sectionUnanchored}},
	}
	seen := map[thread]bool{}
	found := map[topicViolation]bool{}
	contracts := map[string]bool{}

	for len(pending) > 0 {
		t := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if seen[t] {
			continue
		}

		seen[t] = true
		inst := &prog.Inst[t.pc]

		follow := func(pc uint32) {
			next := t
			next.pc = pc
			pending = append(pending, next)
		}

		switch inst.Op {
		case syntax.InstAlt, syntax.InstAltMatch:
			follow(inst.Out)
			follow(inst.Arg)
		case syntax.InstCapture, syntax.InstNop:
			follow(inst.Out)
		case syntax.InstEmptyWidth:
			op := syntax.EmptyOp(inst.Arg)
			if op&(syntax.EmptyBeginText|syntax.EmptyBeginLine) != 0 && !t.textStart {
				continue
			}

			if op&(syntax.EmptyEndText|syntax.EmptyEndLine) != 0 {
				t.textEnd = true
			}

			follow(inst.Out)
		case syntax.InstMatch:
			found[t.at.violation()] = true
		case syntax.InstRune, syntax.InstRune1, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
			if t.textEnd {
				continue
			}

			for _, r := range g.candidates(inst) {
				at := g.step(t.at, r)
				if at.section == sectionVirtualPath {
					contracts[at.contract] = true

					continue
				}

				pending = append(pending, thread{pc: inst.Out, at: at})
			}
		case syntax.InstFail:
		}
	}

	bases := map[string]bool{}
	for contract := range contracts {
		bases[dataContractVersionSuffix.ReplaceAllString(contract, "")] = true
	}

	switch {
	case found[topicUnanchored]:
		return topicUnanchored
	case found[topicOutsideLocation]:
		return topicOutsideLocation
	case found[topicNoDataContract], len(bases) > 1:
		return topicNoDataContract
	default:
		return topicAllowed
	}
}

// candidates returns the runes inst consumes that the grammar tells apart: every
// significant rune it accepts plus one insignificant rune per accepted range.
func (g topicGrammar) candidates(inst *syntax.Inst) []rune {
	ranges := []rune{0, unicode.MaxRune}
	accepts := func(rune) bool { return true }

	switch inst.Op {
	case syntax.InstRune1:
		return []rune{inst.Rune[0]}
	case syntax.InstRune:
		ranges = inst.Rune
		if len(ranges) == 1 {
			ranges = []rune{ranges[0], ranges[0]}
		}

		accepts = inst.MatchRune
	}

	var out []rune

	for r := range g.significant {
		if r != '\n' && accepts(r) {
			out = append(out, r)
		}
	}

	for i := 0; i+1 < len(ranges); i += 2 {
		for r := ranges[i]; r <= ranges[i+1]; r++ {
			if r != '\n' && !g.significant[r] {
				out = append(out, r)

				break
			}
		}
	}

	// A case-folded single rune also accepts the other runes of its folding orbit.
	if inst.Op == syntax.InstRune && len(inst.Rune) == 1 && syntax.Flags(inst.Arg)&syntax.FoldCase != 0 {
		for r := unicode.SimpleFold(inst.Rune[0]); r != inst.Rune[0]; r = unicode.SimpleFold(r) {
			out = append(out, r)
		}
	}

	return out
}
