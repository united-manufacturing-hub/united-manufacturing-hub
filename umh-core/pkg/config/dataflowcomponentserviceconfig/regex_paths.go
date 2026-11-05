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

// compileRegex compiles pattern into the program regexp.Compile runs.
func compileRegex(pattern string) (*syntax.Prog, error) {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, err
	}

	return syntax.Compile(parsed.Simplify())
}

// isAnchored reports whether every match of program starts at the start of the text.
// StartCond only sees a ^ in front of every branch, so ^a|^b is not anchored and
// ^(?:a|b) is.
func isAnchored(program *syntax.Prog) bool {
	return program.StartCond()&(syntax.EmptyBeginText|syntax.EmptyBeginLine) != 0
}

// regexPosition is an instruction of the regex program, the grammar state reached there,
// and whether the text start or end has been asserted.
type regexPosition struct {
	instruction uint32
	state       writeTopicState
	atTextStart bool
	atTextEnd   bool
}

// matchEndStates follows every path through program from the start of a text without
// newlines and returns the grammar state after every prefix of the text it matches. The
// text may continue past that prefix, because the match is not anchored at the end.
func matchEndStates(program *syntax.Prog, grammar writeTopicGrammar) []writeTopicState {
	alphabet := newRuneAlphabet(grammar.significant())
	pending := []regexPosition{{
		instruction: uint32(program.Start),
		state:       grammar.start(),
		atTextStart: true,
	}}
	visited := map[regexPosition]bool{}
	var endStates []writeTopicState

	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if visited[current] {
			continue
		}

		visited[current] = true

		instruction := &program.Inst[current.instruction]
		if instruction.Op == syntax.InstMatch {
			if !slices.Contains(endStates, current.state) {
				endStates = append(endStates, current.state)
			}

			continue
		}

		pending = append(pending, successors(current, instruction, alphabet, grammar)...)
	}

	return endStates
}

// successors returns the positions reachable from current in one instruction.
func successors(current regexPosition, instruction *syntax.Inst, alphabet runeAlphabet, grammar writeTopicGrammar) []regexPosition {
	switch instruction.Op {
	case syntax.InstAlt, syntax.InstAltMatch:
		return []regexPosition{current.at(instruction.Out), current.at(instruction.Arg)}
	case syntax.InstCapture, syntax.InstNop:
		return []regexPosition{current.at(instruction.Out)}
	case syntax.InstEmptyWidth:
		return passAssertion(current, instruction)
	case syntax.InstRune, syntax.InstRune1, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
		return consumeRune(current, instruction, alphabet, grammar)
	default:
		return nil
	}
}

func (p regexPosition) at(instruction uint32) regexPosition {
	p.instruction = instruction

	return p
}

// passAssertion handles ^, $ and their multi-line forms. It treats other assertions such
// as \b as always true, which only adds paths, so no text the regex matches is missed.
func passAssertion(current regexPosition, instruction *syntax.Inst) []regexPosition {
	assertion := syntax.EmptyOp(instruction.Arg)

	// The text contains no newline, so ^ and $ in multi-line mode hold only at the ends.
	if assertion&(syntax.EmptyBeginText|syntax.EmptyBeginLine) != 0 && !current.atTextStart {
		return nil
	}

	next := current.at(instruction.Out)
	if assertion&(syntax.EmptyEndText|syntax.EmptyEndLine) != 0 {
		next.atTextEnd = true
	}

	return []regexPosition{next}
}

func consumeRune(current regexPosition, instruction *syntax.Inst, alphabet runeAlphabet, grammar writeTopicGrammar) []regexPosition {
	if current.atTextEnd {
		return nil
	}

	var next []regexPosition

	for _, r := range alphabet.representatives(instruction) {
		next = append(next, regexPosition{
			instruction: instruction.Out,
			state:       grammar.step(current.state, r),
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
// insignificant rune when it accepts any. The text contains no newline, so none is returned.
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
