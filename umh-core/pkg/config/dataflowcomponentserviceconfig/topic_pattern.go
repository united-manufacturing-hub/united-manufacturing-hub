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
	"regexp/syntax"
	"slices"
	"unicode"
)

// topicGrammar is a deterministic automaton over the runes of a UNS topic.
type topicGrammar[S comparable] interface {
	// Start is the state before the first rune of a topic.
	Start() S
	// Step is the state after reading r in state.
	Step(state S, r rune) S
	// Significant lists the runes Step treats differently from the rest. Step must give
	// every rune outside this list the same transition, because the search tries only one
	// of them.
	Significant() []rune
}

// topicAnalysis describes every topic a pattern can match.
type topicAnalysis[S comparable] struct {
	// EndStates are the grammar states after every topic prefix the pattern matches.
	// The topic may continue past that prefix, because the match is not anchored at
	// the end.
	EndStates []S
	// MatchesMidTopic reports whether the pattern also matches a topic starting past
	// its first rune, which happens when the pattern does not start with ^.
	MatchesMidTopic bool
}

// analyzeTopicPattern returns what pattern can match, read against grammar.
func analyzeTopicPattern[S comparable](pattern string, grammar topicGrammar[S]) (topicAnalysis[S], error) {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return topicAnalysis[S]{}, err
	}

	program, err := syntax.Compile(parsed.Simplify())
	if err != nil {
		return topicAnalysis[S]{}, err
	}

	alphabet := newRuneAlphabet(grammar.Significant())

	return topicAnalysis[S]{
		EndStates:       walkPattern(program, alphabet, grammar, true),
		MatchesMidTopic: len(walkPattern(program, alphabet, anyTopicGrammar{}, false)) > 0,
	}, nil
}

// patternPosition is an instruction of the regex program, the grammar state reached there,
// and whether the topic start or end has been asserted.
type patternPosition[S comparable] struct {
	instruction uint32
	state       S
	atTextStart bool
	atTextEnd   bool
}

// walkPattern follows every path through program and returns the grammar state at each match.
func walkPattern[S comparable](program *syntax.Prog, alphabet runeAlphabet, grammar topicGrammar[S], atTextStart bool) []S {
	pending := []patternPosition[S]{{
		instruction: uint32(program.Start),
		state:       grammar.Start(),
		atTextStart: atTextStart,
	}}
	visited := map[patternPosition[S]]bool{}
	var endStates []S

	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if visited[current] {
			continue
		}

		visited[current] = true

		instruction := &program.Inst[current.instruction]
		if instruction.Op == syntax.InstMatch {
			endStates = appendUnique(endStates, current.state)

			continue
		}

		pending = append(pending, successors(current, instruction, alphabet, grammar)...)
	}

	return endStates
}

// successors returns the positions reachable from current in one instruction.
func successors[S comparable](current patternPosition[S], instruction *syntax.Inst, alphabet runeAlphabet, grammar topicGrammar[S]) []patternPosition[S] {
	switch instruction.Op {
	case syntax.InstAlt, syntax.InstAltMatch:
		return []patternPosition[S]{current.at(instruction.Out), current.at(instruction.Arg)}
	case syntax.InstCapture, syntax.InstNop:
		return []patternPosition[S]{current.at(instruction.Out)}
	case syntax.InstEmptyWidth:
		return passAssertion(current, instruction)
	case syntax.InstRune, syntax.InstRune1, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
		return consumeRune(current, instruction, alphabet, grammar)
	default:
		return nil
	}
}

func (p patternPosition[S]) at(instruction uint32) patternPosition[S] {
	p.instruction = instruction

	return p
}

// passAssertion handles ^, $ and their multi-line forms. It treats other assertions such
// as \b as always true, which only adds paths, so no topic the regex matches is missed.
func passAssertion[S comparable](current patternPosition[S], instruction *syntax.Inst) []patternPosition[S] {
	assertion := syntax.EmptyOp(instruction.Arg)

	// Topics contain no newline, so ^ and $ in multi-line mode hold only at the ends.
	if assertion&(syntax.EmptyBeginText|syntax.EmptyBeginLine) != 0 && !current.atTextStart {
		return nil
	}

	next := current.at(instruction.Out)
	if assertion&(syntax.EmptyEndText|syntax.EmptyEndLine) != 0 {
		next.atTextEnd = true
	}

	return []patternPosition[S]{next}
}

func consumeRune[S comparable](current patternPosition[S], instruction *syntax.Inst, alphabet runeAlphabet, grammar topicGrammar[S]) []patternPosition[S] {
	if current.atTextEnd {
		return nil
	}

	var next []patternPosition[S]

	for _, r := range alphabet.representatives(instruction) {
		next = append(next, patternPosition[S]{
			instruction: instruction.Out,
			state:       grammar.Step(current.state, r),
		})
	}

	return next
}

// runeAlphabet picks the runes to try for an instruction: the significant runes it
// accepts, plus one insignificant rune that stands for all others it accepts.
type runeAlphabet struct {
	significant []rune
}

func newRuneAlphabet(significant []rune) runeAlphabet {
	return runeAlphabet{significant: significant}
}

func (a runeAlphabet) isSignificant(r rune) bool {
	return slices.Contains(a.significant, r)
}

// representatives returns every significant rune instruction accepts, plus one
// insignificant rune when it accepts any. Topics contain no newline, so none is returned.
func (a runeAlphabet) representatives(instruction *syntax.Inst) []rune {
	var out []rune

	for _, r := range a.significant {
		if r != '\n' && instruction.MatchRune(r) {
			out = append(out, r)
		}
	}

	standIn, found := a.insignificantStandIn(instruction)
	if found {
		out = append(out, standIn)
	}

	return out
}

func (a runeAlphabet) insignificantStandIn(instruction *syntax.Inst) (rune, bool) {
	switch {
	case instruction.Op == syntax.InstRune1:
		return a.firstInsignificant([]rune{instruction.Rune[0]})
	case instruction.Op == syntax.InstRuneAny, instruction.Op == syntax.InstRuneAnyNotNL:
		return a.firstInsignificantInRange(0, unicode.MaxRune)
	case len(instruction.Rune) == 1:
		// A case-insensitive single rune is stored once but also accepts its case variants.
		return a.firstInsignificant(caseVariants(instruction.Rune[0]))
	}

	for i := 0; i+1 < len(instruction.Rune); i += 2 {
		standIn, found := a.firstInsignificantInRange(instruction.Rune[i], instruction.Rune[i+1])
		if found {
			return standIn, true
		}
	}

	return 0, false
}

func (a runeAlphabet) firstInsignificant(runes []rune) (rune, bool) {
	for _, r := range runes {
		if r != '\n' && !a.isSignificant(r) {
			return r, true
		}
	}

	return 0, false
}

// firstInsignificantInRange returns within len(a.significant)+2 iterations, because it
// skips only significant runes and the newline, so a range up to unicode.MaxRune is cheap.
func (a runeAlphabet) firstInsignificantInRange(low, high rune) (rune, bool) {
	for r := low; r <= high; r++ {
		if r != '\n' && !a.isSignificant(r) {
			return r, true
		}
	}

	return 0, false
}

func caseVariants(r rune) []rune {
	variants := []rune{r}
	for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
		variants = append(variants, folded)
	}

	return variants
}

// anyTopicGrammar is the grammar with one state, used to ask only whether a match exists.
type anyTopicGrammar struct{}

func (anyTopicGrammar) Start() struct{}              { return struct{}{} }
func (anyTopicGrammar) Step(struct{}, rune) struct{} { return struct{}{} }
func (anyTopicGrammar) Significant() []rune          { return nil }

func appendUnique[S comparable](states []S, state S) []S {
	if slices.Contains(states, state) {
		return states
	}

	return append(states, state)
}
