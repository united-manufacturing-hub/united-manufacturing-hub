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

package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// AnalyzeRecovery analyzes FSM recovery issues between sessions.
func (a *LogAnalyzer) AnalyzeRecovery() {
	fmt.Println("")
	fmt.Println("=== Recovery Analysis ===")
	fmt.Println("")

	if len(a.Sessions) < 2 {
		fmt.Println("Single session detected. No recovery analysis needed.")

		return
	}

	// Analyze FSM states at end of first session and start of second
	fmt.Println("Checking FSM states across restart...")
	fmt.Println("")

	// Find FSMs that were active before restart
	activeBeforeRestart := a.findFSMsAtTime(a.Sessions[0].EndTime.Add(-time.Second))
	activeAfterRestart := a.findFSMsAtTime(a.Sessions[1].StartTime.Add(time.Second * 5))

	// Find FSMs that didn't recover
	notRecovered := make(map[string]string)

	for fsmName, beforeState := range activeBeforeRestart {
		afterState, exists := activeAfterRestart[fsmName]
		if !exists {
			notRecovered[fsmName] = fmt.Sprintf("was %s, not found after restart", beforeState)
		} else if isFailedState(afterState) {
			notRecovered[fsmName] = fmt.Sprintf("was %s, now %s", beforeState, afterState)
		}
	}

	if len(notRecovered) > 0 {
		fmt.Println("❌ FSMs that failed to recover properly:")

		for fsm, issue := range notRecovered {
			fmt.Printf("  • %s: %s\n", fsm, issue)
		}
	} else {
		fmt.Println("✓ All FSMs recovered successfully")
	}

	// Check for persistent errors across sessions
	a.checkPersistentErrors()

	// Analyze agent-specific issues
	a.analyzeAgentRecovery()
}

func (a *LogAnalyzer) findFSMsAtTime(t time.Time) map[string]string {
	states := make(map[string]string)

	for fsmName, history := range a.FSMHistories {
		if history == nil {
			continue
		}

		var lastState string

		for _, transition := range history.Transitions {
			if transition.Timestamp.Before(t) && transition.Success {
				lastState = transition.ToState
			}
		}

		if lastState != "" {
			states[fsmName] = lastState
		}
	}

	return states
}

func isFailedState(state string) bool {
	failedStates := []string{"failed", "error", "removing", "to_be_removed"}

	stateLower := strings.ToLower(state)
	for _, failed := range failedStates {
		if strings.Contains(stateLower, failed) {
			return true
		}
	}

	return false
}

func (a *LogAnalyzer) checkPersistentErrors() {
	fmt.Println("\n=== Persistent Errors Across Sessions ===")

	// Group errors by pattern
	session1Errors := make(map[string]int)
	session2Errors := make(map[string]int)

	for _, err := range a.Errors {
		key := extractErrorKey(err)
		if err.Timestamp.Before(a.Sessions[0].EndTime) {
			session1Errors[key]++
		} else {
			session2Errors[key]++
		}
	}

	// Find errors that appear in both sessions
	persistent := make([]string, 0)

	for errType := range session1Errors {
		if _, exists := session2Errors[errType]; exists {
			persistent = append(persistent, errType)
		}
	}

	if len(persistent) > 0 {
		fmt.Println("\nErrors that persist across restart:")
		sort.Strings(persistent)

		for _, err := range persistent {
			fmt.Printf("  • %s (Session 1: %d, Session 2: %d)\n",
				err, session1Errors[err], session2Errors[err])
		}
	} else {
		fmt.Println("\nNo persistent errors found across sessions.")
	}
}

func (a *LogAnalyzer) analyzeAgentRecovery() {
	fmt.Println("\n=== Agent Recovery Analysis ===")

	// Look for agent-related patterns
	agentErrors := 0
	agentNotFound := 0

	for _, err := range a.Errors {
		if strings.Contains(err.Message, "agent") {
			agentErrors++

			if strings.Contains(err.Message, "instance not found") {
				agentNotFound++
			}
		}
	}

	fmt.Printf("\nAgent-related errors: %d\n", agentErrors)
	fmt.Printf("Agent instance not found errors: %d\n", agentNotFound)

	// Check if agent FSM exists
	agentFSMs := make([]string, 0)

	for fsmName := range a.FSMHistories {
		if strings.Contains(strings.ToLower(fsmName), "agent") {
			agentFSMs = append(agentFSMs, fsmName)
		}
	}

	if len(agentFSMs) == 0 {
		fmt.Println("\n⚠️  No agent FSM found in logs!")
		fmt.Println("This explains why 'agent instance not found' errors occur.")
		fmt.Println("The agent monitor FSM may not be creating the agent instance properly.")
	} else {
		fmt.Printf("\nAgent FSMs found: %v\n", agentFSMs)

		for _, fsm := range agentFSMs {
			a.ShowFSMHistory(fsm)
		}
	}
}

// CompareSessionStates compares FSM states between two sessions.
func (a *LogAnalyzer) CompareSessionStates() {
	if len(a.Sessions) < 2 {
		fmt.Println("Need at least 2 sessions to compare.")

		return
	}

	fmt.Println("")
	fmt.Println("=== Session State Comparison ===")
	fmt.Println("")

	// Get final states from first session
	session1End := a.Sessions[0].EndTime.Add(-time.Second)
	session1States := a.findFSMsAtTime(session1End)

	// Get initial states from second session
	session2Start := a.Sessions[1].StartTime.Add(time.Second * 5)
	session2States := a.findFSMsAtTime(session2Start)

	fmt.Printf("Session 1 end FSMs: %d\n", len(session1States))
	fmt.Printf("Session 2 start FSMs: %d\n\n", len(session2States))

	// Compare
	allFSMs := make(map[string]bool)
	for fsm := range session1States {
		allFSMs[fsm] = true
	}

	for fsm := range session2States {
		allFSMs[fsm] = true
	}

	fmt.Println("FSM State Changes:")
	fmt.Printf("%-30s %-20s %-20s\n", "FSM", "Session 1 End", "Session 2 Start")
	fmt.Println(strings.Repeat("-", 72))

	fsmList := make([]string, 0, len(allFSMs))
	for fsm := range allFSMs {
		fsmList = append(fsmList, fsm)
	}

	sort.Strings(fsmList)

	for _, fsm := range fsmList {
		state1 := session1States[fsm]
		state2 := session2States[fsm]

		if state1 == "" {
			state1 = "not found"
		}

		if state2 == "" {
			state2 = "not found"
		}

		marker := " "

		if state1 != state2 {
			if state2 == "not found" {
				marker = "❌"
			} else if state1 == "not found" {
				marker = "✓"
			} else {
				marker = "⚠️"
			}
		}

		fmt.Printf("%s %-28s %-20s %-20s\n", marker, fsm, state1, state2)
	}
}
