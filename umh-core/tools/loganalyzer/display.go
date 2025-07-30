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

func (a *LogAnalyzer) ShowTickTimeline(startTick, endTick int) {
	fmt.Printf("\n=== Tick Timeline (Ticks %d-%d) ===\n\n", startTick, endTick)

	ticks := make([]int, 0)
	for tick := range a.TickMap {
		if tick >= startTick && tick <= endTick {
			ticks = append(ticks, tick)
		}
	}
	sort.Ints(ticks)

	for _, tick := range ticks {
		tickData := a.TickMap[tick]
		fmt.Printf("Tick %d (%s):\n", tick, tickData.Timestamp.Format("15:04:05.000"))
		
		fsmEvents := make([]LogEntry, 0)
		reconciliations := make([]LogEntry, 0)
		desiredStates := make([]LogEntry, 0)
		actions := make([]LogEntry, 0)
		errors := make([]LogEntry, 0)
		
		for _, event := range tickData.Events {
			switch event.EntryType {
			case EntryTypeFSMTransition, EntryTypeFSMAttempt, EntryTypeFSMFailed:
				fsmEvents = append(fsmEvents, event)
			case EntryTypeReconciliation:
				reconciliations = append(reconciliations, event)
			case EntryTypeFSMDesiredState:
				desiredStates = append(desiredStates, event)
			case EntryTypeActionStart, EntryTypeActionDone:
				actions = append(actions, event)
			case EntryTypeError:
				errors = append(errors, event)
			}
		}

		if len(desiredStates) > 0 {
			fmt.Println("  Desired State Changes:")
			for _, event := range desiredStates {
				fmt.Printf("    • %s → %s\n", event.FSMName, event.DesiredState)
			}
		}

		if len(fsmEvents) > 0 {
			fmt.Println("  FSM Transitions:")
			for _, event := range fsmEvents {
				status := "✓"
				if event.EntryType == EntryTypeFSMAttempt {
					status = "→"
				} else if event.EntryType == EntryTypeFSMFailed {
					status = "✗"
				}
				toState := event.ToState
				if toState == "" {
					toState = "?"
				}
				fmt.Printf("    %s %s: %s → %s (event: %s)\n", 
					status, event.FSMName, event.FromState, 
					toState, event.Event)
			}
		}

		if len(reconciliations) > 0 {
			fmt.Println("  Reconciliations:")
			for _, event := range reconciliations {
				fmt.Printf("    • %s\n", event.Component)
			}
		}
		
		if len(actions) > 0 {
			fmt.Println("  Actions:")
			for _, event := range actions {
				marker := "→"
				if event.EntryType == EntryTypeActionDone {
					marker = "✓"
				}
				fmt.Printf("    %s %s\n", marker, event.Message)
			}
		}
		
		if len(errors) > 0 {
			fmt.Println("  Errors:")
			for _, event := range errors {
				fmt.Printf("    ✗ %s\n", event.Message)
			}
		}
		
		fmt.Println()
	}
}

func (a *LogAnalyzer) ShowFSMHistory(fsmName string) {
	history, exists := a.FSMHistories[fsmName]
	if !exists {
		fmt.Printf("No history found for FSM: %s\n", fsmName)
		return
	}

	fmt.Printf("\n=== FSM History: %s ===\n\n", fsmName)
	
	currentState := ""
	for i, transition := range history.Transitions {
		if i == 0 || transition.FromState != currentState {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("State: %s\n", transition.FromState)
		}
		
		arrow := "→"
		if transition.Success {
			arrow = "✓→"
			currentState = transition.ToState
		} else {
			arrow = "✗→"
		}
		
		fmt.Printf("  Tick %d: %s %s (event: %s, desired: %s)\n",
			transition.Tick, arrow, transition.ToState, 
			transition.Event, transition.DesiredState)
	}
	
	if currentState != "" {
		fmt.Printf("\nCurrent State: %s\n", currentState)
	}
}

func (a *LogAnalyzer) ListFSMs() {
	fmt.Println("")
	fmt.Println("=== FSM List ===")
	fmt.Println("")
	
	fsms := make([]string, 0, len(a.FSMHistories))
	for name := range a.FSMHistories {
		fsms = append(fsms, name)
	}
	sort.Strings(fsms)

	maxNameLen := 0
	for _, name := range fsms {
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}

	fmt.Printf("%-*s  Transitions  Last State\n", maxNameLen, "FSM Name")
	fmt.Println(strings.Repeat("-", maxNameLen+30))
	
	for _, name := range fsms {
		history := a.FSMHistories[name]
		lastState := "unknown"
		
		for i := len(history.Transitions) - 1; i >= 0; i-- {
			if history.Transitions[i].Success {
				lastState = history.Transitions[i].ToState
				break
			}
		}
		
		fmt.Printf("%-*s  %11d  %s\n", maxNameLen, name, 
			len(history.Transitions), lastState)
	}
}

func (a *LogAnalyzer) FindStuckFSMs() {
	fmt.Println("")
	fmt.Println("=== Potentially Stuck FSMs ===")
	fmt.Println("")
	
	stuck := make([]string, 0)
	
	for name, history := range a.FSMHistories {
		if len(history.Transitions) == 0 {
			continue
		}

		failedAttempts := 0
		for i := len(history.Transitions) - 1; i >= 0 && i >= len(history.Transitions)-5; i-- {
			if !history.Transitions[i].Success {
				failedAttempts++
			} else {
				break
			}
		}
		
		if failedAttempts >= 3 {
			stuck = append(stuck, name)
		}
	}
	
	if len(stuck) == 0 {
		fmt.Println("No stuck FSMs detected.")
		return
	}
	
	for _, name := range stuck {
		history := a.FSMHistories[name]
		lastTransition := history.Transitions[len(history.Transitions)-1]
		
		fmt.Printf("FSM: %s\n", name)
		fmt.Printf("  Stuck in state: %s\n", lastTransition.FromState)
		fmt.Printf("  Trying to reach: %s\n", lastTransition.DesiredState)
		fmt.Printf("  Failed attempts: %d\n", countRecentFailures(history))
		fmt.Printf("  Last attempt: Tick %d\n\n", lastTransition.Tick)
	}
}

func countRecentFailures(history *FSMHistory) int {
	count := 0
	for i := len(history.Transitions) - 1; i >= 0; i-- {
		if !history.Transitions[i].Success {
			count++
		} else {
			break
		}
	}
	return count
}

func (a *LogAnalyzer) ShowSummary() {
	fmt.Println("")
	fmt.Println("=== Log Analysis Summary ===")
	fmt.Println("")
	
	totalTicks := len(a.TickMap)
	totalFSMs := len(a.FSMHistories)
	totalTransitions := 0
	failedTransitions := 0
	
	for _, history := range a.FSMHistories {
		for _, transition := range history.Transitions {
			totalTransitions++
			if !transition.Success {
				failedTransitions++
			}
		}
	}
	
	fmt.Printf("Total Entries:     %d\n", len(a.Entries))
	fmt.Printf("Total Errors:      %d\n", len(a.Errors))
	fmt.Printf("Total Ticks:       %d\n", totalTicks)
	fmt.Printf("Total FSMs:        %d\n", totalFSMs)
	fmt.Printf("Total Transitions: %d\n", totalTransitions)
	fmt.Printf("Failed Attempts:   %d (%.1f%%)\n", 
		failedTransitions, 
		float64(failedTransitions)/float64(totalTransitions)*100)
	
	// Show multiple starts
	if len(a.Starts) > 1 {
		fmt.Printf("\n⚠️  Multiple UMH Core starts detected: %d\n", len(a.Starts))
		fmt.Println("\nSessions:")
		for i, session := range a.Sessions {
			duration := session.EndTime.Sub(session.StartTime)
			fmt.Printf("  Session %d: %s - %s (duration: %s)\n", 
				i+1, 
				session.StartTime.Format("15:04:05"),
				session.EndTime.Format("15:04:05"),
				duration.Round(time.Second))
			if session.StartTick >= 0 && session.EndTick >= 0 {
				fmt.Printf("    Ticks: %d - %d\n", session.StartTick, session.EndTick)
			}
			fmt.Printf("    Errors: %d\n", session.Errors)
		}
	} else if len(a.Starts) == 1 {
		fmt.Printf("\nSingle session detected\n")
	}
	
	if totalTicks > 0 {
		ticks := make([]int, 0, len(a.TickMap))
		for tick := range a.TickMap {
			ticks = append(ticks, tick)
		}
		sort.Ints(ticks)
		
		firstTick := a.TickMap[ticks[0]]
		lastTick := a.TickMap[ticks[len(ticks)-1]]
		
		fmt.Printf("\nOverall Time Range: %s - %s\n", 
			firstTick.Timestamp.Format("15:04:05"),
			lastTick.Timestamp.Format("15:04:05"))
		fmt.Printf("Overall Tick Range: %d - %d\n", ticks[0], ticks[len(ticks)-1])
	}
}