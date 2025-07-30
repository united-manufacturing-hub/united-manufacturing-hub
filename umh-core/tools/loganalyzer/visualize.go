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
)

func (a *LogAnalyzer) ShowVisualTimeline(startTick, endTick int) {
	fmt.Printf("\n=== Visual FSM Timeline (Ticks %d-%d) ===\n\n", startTick, endTick)

	fsmNames := make([]string, 0, len(a.FSMHistories))
	for name := range a.FSMHistories {
		fsmNames = append(fsmNames, name)
	}
	sort.Strings(fsmNames)

	maxNameLen := 0
	for _, name := range fsmNames {
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}

	fmt.Printf("%-*s ", maxNameLen, "FSM/Tick")
	for tick := startTick; tick <= endTick && tick <= a.CurrentTick; tick++ {
		fmt.Printf("%3d ", tick)
	}
	fmt.Println()
	
	fmt.Print(strings.Repeat("-", maxNameLen+1))
	for tick := startTick; tick <= endTick && tick <= a.CurrentTick; tick++ {
		fmt.Print("----")
	}
	fmt.Println()

	for _, fsmName := range fsmNames {
		history := a.FSMHistories[fsmName]
		fmt.Printf("%-*s ", maxNameLen, truncateName(fsmName, maxNameLen))
		
		currentState := getInitialState(history)
		stateChanged := false
		
		for tick := startTick; tick <= endTick && tick <= a.CurrentTick; tick++ {
			symbol := " . "
			newState := currentState
			
			for _, transition := range history.Transitions {
				if transition.Tick == tick {
					if transition.Success {
						symbol = " → "
						newState = transition.ToState
						stateChanged = true
					} else {
						symbol = " ✗ "
					}
					break
				}
			}
			
			if tick == startTick && currentState != "" {
				symbol = fmt.Sprintf("%-3s", getStateSymbol(currentState))
			} else if stateChanged && newState != currentState {
				symbol = fmt.Sprintf("%-3s", getStateSymbol(newState))
				currentState = newState
				stateChanged = false
			}
			
			fmt.Print(symbol + " ")
		}
		fmt.Println()
	}
	
	fmt.Println("\nLegend: . = no change, → = transition, ✗ = failed attempt")
	fmt.Println("States: C=created, S=stopped, R=running, A=active, M=monitoring")
}

func getInitialState(history *FSMHistory) string {
	if len(history.Transitions) > 0 {
		return history.Transitions[0].FromState
	}
	return ""
}

func getStateSymbol(state string) string {
	switch {
	case strings.Contains(state, "created"):
		return "C"
	case strings.Contains(state, "stopped"):
		return "S"
	case strings.Contains(state, "running"):
		return "R"
	case strings.Contains(state, "active"):
		return "A"
	case strings.Contains(state, "monitoring"):
		return "M"
	case strings.Contains(state, "up"):
		return "U"
	case strings.Contains(state, "down"):
		return "D"
	case strings.Contains(state, "creating"):
		return "c"
	case strings.Contains(state, "starting"):
		return "s"
	case strings.Contains(state, "stopping"):
		return "x"
	default:
		return "?"
	}
}

func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-3] + "..."
}

func (a *LogAnalyzer) ShowConcurrentActivity() {
	fmt.Println("")
	fmt.Println("=== Concurrent FSM Activity Analysis ===")
	fmt.Println("")
	
	tickActivity := make(map[int][]string)
	
	for fsmName, history := range a.FSMHistories {
		for _, transition := range history.Transitions {
			if tickActivity[transition.Tick] == nil {
				tickActivity[transition.Tick] = make([]string, 0)
			}
			
			activity := fmt.Sprintf("%s: %s→%s", 
				fsmName, transition.FromState, transition.ToState)
			if !transition.Success {
				activity = fmt.Sprintf("%s (failed)", activity)
			}
			
			tickActivity[transition.Tick] = append(tickActivity[transition.Tick], activity)
		}
	}
	
	ticks := make([]int, 0, len(tickActivity))
	for tick := range tickActivity {
		ticks = append(ticks, tick)
	}
	sort.Ints(ticks)
	
	maxConcurrent := 0
	maxConcurrentTick := 0
	
	for _, tick := range ticks {
		activities := tickActivity[tick]
		if len(activities) > maxConcurrent {
			maxConcurrent = len(activities)
			maxConcurrentTick = tick
		}
		
		if len(activities) > 3 {
			fmt.Printf("Tick %d: %d concurrent transitions\n", tick, len(activities))
			for _, activity := range activities {
				fmt.Printf("  • %s\n", activity)
			}
			fmt.Println()
		}
	}
	
	fmt.Printf("Peak concurrent activity: %d transitions at tick %d\n", 
		maxConcurrent, maxConcurrentTick)
}