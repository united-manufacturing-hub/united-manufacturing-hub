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

func (a *LogAnalyzer) ShowErrors() {
	fmt.Printf("\n=== Error Analysis (%d total errors) ===\n\n", len(a.Errors))

	if len(a.Errors) == 0 {
		fmt.Println("No errors found in the log file.")

		return
	}

	// Group errors by message pattern
	errorGroups := make(map[string][]LogEntry)

	for _, err := range a.Errors {
		// Extract key parts of error for grouping
		key := extractErrorKey(err)
		errorGroups[key] = append(errorGroups[key], err)
	}

	// Sort groups by frequency
	type errorGroup struct {
		Key   string
		First LogEntry
		Last  LogEntry
		Count int
	}

	groups := make([]errorGroup, 0, len(errorGroups))
	for key, errors := range errorGroups {
		group := errorGroup{
			Key:   key,
			Count: len(errors),
			First: errors[0],
			Last:  errors[len(errors)-1],
		}
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Count > groups[j].Count
	})

	// Show top error groups
	fmt.Println("Error Summary (grouped by type):")

	for i, group := range groups {
		if i >= 10 && len(groups) > 10 {
			fmt.Printf("\n... and %d more error types\n", len(groups)-10)

			break
		}

		fmt.Printf("\n%d. %s (count: %d)\n", i+1, group.Key, group.Count)
		fmt.Printf("   First: %s\n", group.First.Timestamp.Format("15:04:05.000"))
		fmt.Printf("   Last:  %s\n", group.Last.Timestamp.Format("15:04:05.000"))

		if group.First.Component != "" {
			fmt.Printf("   Component: %s\n", group.First.Component)
		}
	}

	// Show errors over time
	a.showErrorTimeline()
}

func extractErrorKey(entry LogEntry) string {
	msg := entry.Message
	// For standard error format, extract the error message
	if entry.EntryType == EntryTypeError {
		return entry.Message
	}
	// Extract error message from log line
	if idx := strings.Index(msg, "Error "); idx >= 0 {
		errorPart := msg[idx:]
		// Remove specific details like IDs, keeping just the pattern
		errorPart = strings.ReplaceAll(errorPart, "agent instance not found", "instance not found")

		return errorPart
	}
	// Fallback to full message
	return msg
}

func (a *LogAnalyzer) showErrorTimeline() {
	fmt.Println("\n=== Error Timeline ===")

	if len(a.Sessions) > 1 {
		fmt.Println("\nErrors by session:")

		for i, session := range a.Sessions {
			fmt.Printf("  Session %d: %d errors\n", i+1, session.Errors)
		}
	}
	// Group errors by tick
	errorsByTick := make(map[int]int)

	for _, err := range a.Errors {
		// Find which tick this error belongs to
		for tick, tickData := range a.TickMap {
			if err.Timestamp.After(tickData.Timestamp.Add(-time.Second)) &&
				err.Timestamp.Before(tickData.Timestamp.Add(time.Second)) {
				errorsByTick[tick]++

				break
			}
		}
	}

	if len(errorsByTick) > 0 {
		fmt.Println("\nTicks with most errors:")

		type tickErrors struct {
			Tick  int
			Count int
		}

		tickList := make([]tickErrors, 0, len(errorsByTick))
		for tick, count := range errorsByTick {
			tickList = append(tickList, tickErrors{Tick: tick, Count: count})
		}

		sort.Slice(tickList, func(i, j int) bool {
			return tickList[i].Count > tickList[j].Count
		})

		for i, te := range tickList {
			if i >= 5 {
				break
			}

			fmt.Printf("  Tick %d: %d errors\n", te.Tick, te.Count)
		}
	}
}

func (a *LogAnalyzer) ShowErrorsForTick(tick int) {
	fmt.Printf("\n=== Errors at Tick %d ===\n\n", tick)

	found := false

	for _, err := range a.Errors {
		// Check if error is near this tick
		if tickData, exists := a.TickMap[tick]; exists {
			if err.Timestamp.After(tickData.Timestamp.Add(-time.Second)) &&
				err.Timestamp.Before(tickData.Timestamp.Add(time.Second)) {
				found = true

				fmt.Printf("[%s] %s\n", err.Timestamp.Format("15:04:05.000"), err.Message)
			}
		}
	}

	if !found {
		fmt.Println("No errors found at this tick.")
	}
}
