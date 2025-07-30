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
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	instanceNotFoundRegex = regexp.MustCompile(`instance (\S+) not found`)
	s6ServiceExistsRegex  = regexp.MustCompile(`S6 service (\S+) (?:exists|already exists)`)
	s6ServiceNotExistRegex = regexp.MustCompile(`S6 service (\S+) does not exist`)
	reconcileErrorRegex   = regexp.MustCompile(`error reconciling external changes:.*instance (\S+) not found`)
	backoffSuspendRegex   = regexp.MustCompile(`Suspending operations for (\d+) ticks.*instance (\S+) not found`)
)

type OrphanedService struct {
	InstanceName string
	Component    string
	FirstSeen    time.Time
	ErrorCount   int
	BackoffTicks int
	ManagerType  string
}

// AnalyzeOrphanedServices finds services that exist in S6 but not in FSM managers
func (a *LogAnalyzer) AnalyzeOrphanedServices() {
	fmt.Println("")
	fmt.Println("=== Orphaned Service Analysis ===")
	fmt.Println("(S6 services that exist but have no FSM manager instance)")
	fmt.Println("")
	
	orphans := make(map[string]*OrphanedService)
	s6Services := make(map[string]bool)
	
	// First pass: collect all "instance not found" errors
	for _, entry := range a.Entries {
		// Check for instance not found
		if match := instanceNotFoundRegex.FindStringSubmatch(entry.Message); match != nil {
			instanceName := match[1]
			if _, exists := orphans[instanceName]; !exists {
				orphans[instanceName] = &OrphanedService{
					InstanceName: instanceName,
					Component:    entry.Component,
					FirstSeen:    entry.Timestamp,
					ManagerType:  extractManagerType(entry.Message),
				}
			}
			orphans[instanceName].ErrorCount++
		}
		
		// Check for backoff suspensions
		if match := backoffSuspendRegex.FindStringSubmatch(entry.Message); match != nil {
			ticks := 0
			fmt.Sscanf(match[1], "%d", &ticks)
			instanceName := match[2]
			if orphan, exists := orphans[instanceName]; exists {
				if ticks > orphan.BackoffTicks {
					orphan.BackoffTicks = ticks
				}
			}
		}
		
		// Track S6 services
		if match := s6ServiceExistsRegex.FindStringSubmatch(entry.Message); match != nil {
			s6Services[match[1]] = true
		}
		if match := s6ServiceNotExistRegex.FindStringSubmatch(entry.Message); match != nil {
			s6Services[match[1]] = false
		}
	}
	
	// Analyze by session
	if len(a.Sessions) >= 2 {
		fmt.Println("Orphaned services after restart:")
		
		session2Start := a.Sessions[1].StartTime
		orphansAfterRestart := make([]*OrphanedService, 0)
		
		for _, orphan := range orphans {
			if orphan.FirstSeen.After(session2Start) {
				orphansAfterRestart = append(orphansAfterRestart, orphan)
			}
		}
		
		// Sort by error count
		sort.Slice(orphansAfterRestart, func(i, j int) bool {
			return orphansAfterRestart[i].ErrorCount > orphansAfterRestart[j].ErrorCount
		})
		
		for _, orphan := range orphansAfterRestart {
			fmt.Printf("\n• %s\n", orphan.InstanceName)
			fmt.Printf("  Component: %s\n", orphan.Component)
			fmt.Printf("  Manager Type: %s\n", orphan.ManagerType)
			fmt.Printf("  Error Count: %d\n", orphan.ErrorCount)
			fmt.Printf("  Max Backoff: %d ticks\n", orphan.BackoffTicks)
			fmt.Printf("  First Error: %s\n", orphan.FirstSeen.Format("15:04:05.000"))
			
			// Check if S6 service exists
			serviceName := guessS6ServiceName(orphan.InstanceName)
			if exists, found := s6Services[serviceName]; found && exists {
				fmt.Printf("  ⚠️  S6 service '%s' exists on filesystem!\n", serviceName)
			}
		}
		
		fmt.Printf("\nTotal orphaned instances after restart: %d\n", len(orphansAfterRestart))
	}
	
	// Show recovery attempts
	a.showRecoveryAttempts()
}

func extractManagerType(message string) string {
	patterns := []struct {
		regex *regexp.Regexp
		typ   string
	}{
		{regexp.MustCompile(`BenthosManager`), "Benthos"},
		{regexp.MustCompile(`ConnectionManager`), "Connection"},
		{regexp.MustCompile(`DataFlowCompManager`), "DataFlowComponent"},
		{regexp.MustCompile(`benthos observed state`), "Benthos"},
		{regexp.MustCompile(`connection observed state`), "Connection"},
		{regexp.MustCompile(`dataflowcomponent`), "DataFlowComponent"},
	}
	
	for _, p := range patterns {
		if p.regex.MatchString(message) {
			return p.typ
		}
	}
	return "Unknown"
}

func guessS6ServiceName(instanceName string) string {
	// Common patterns for S6 service names
	if strings.HasPrefix(instanceName, "topicbrowser-") {
		return "benthos-" + instanceName
	}
	if strings.HasPrefix(instanceName, "protocolconverter-") {
		return "nmap-connection-" + instanceName
	}
	if strings.HasPrefix(instanceName, "dfc-") {
		return "benthos-dataflow-" + instanceName
	}
	return instanceName
}

func (a *LogAnalyzer) showRecoveryAttempts() {
	fmt.Println("\n=== Recovery Attempt Analysis ===")
	
	// Look for patterns where system tries to recover
	addingToManagerRegex := regexp.MustCompile(`Adding (\S+) service (\S+) to (\S+) manager`)
	creatingServiceRegex := regexp.MustCompile(`Creating (\S+) service (\S+)`)
	
	recoveryAttempts := make(map[string][]string)
	
	for _, entry := range a.Entries {
		if len(a.Sessions) >= 2 && entry.Timestamp.After(a.Sessions[1].StartTime) {
			if match := addingToManagerRegex.FindStringSubmatch(entry.Message); match != nil {
				key := fmt.Sprintf("%s:%s", match[1], match[2])
				recoveryAttempts[key] = append(recoveryAttempts[key], 
					fmt.Sprintf("Added to %s manager at %s", match[3], entry.Timestamp.Format("15:04:05.000")))
			}
			if match := creatingServiceRegex.FindStringSubmatch(entry.Message); match != nil {
				key := fmt.Sprintf("%s:%s", match[1], match[2])
				recoveryAttempts[key] = append(recoveryAttempts[key],
					fmt.Sprintf("Created at %s", entry.Timestamp.Format("15:04:05.000")))
			}
		}
	}
	
	if len(recoveryAttempts) == 0 {
		fmt.Println("\nNo recovery attempts found after restart!")
		fmt.Println("⚠️  This suggests the system is not detecting or recovering orphaned services.")
	} else {
		fmt.Printf("\nFound %d recovery attempts:\n", len(recoveryAttempts))
		for service, attempts := range recoveryAttempts {
			fmt.Printf("\n%s:\n", service)
			for _, attempt := range attempts {
				fmt.Printf("  • %s\n", attempt)
			}
		}
	}
}

// ShowInstanceNotFoundTimeline shows when instance not found errors occur
func (a *LogAnalyzer) ShowInstanceNotFoundTimeline() {
	fmt.Println("\n=== Instance Not Found Timeline ===")
	
	// Group by tick
	errorsByTick := make(map[int][]string)
	
	for _, entry := range a.Entries {
		if match := instanceNotFoundRegex.FindStringSubmatch(entry.Message); match != nil {
			// Find the tick this belongs to
			tick := a.findTickForTimestamp(entry.Timestamp)
			if tick > 0 {
				errorsByTick[tick] = append(errorsByTick[tick], match[1])
			}
		}
	}
	
	// Show timeline
	ticks := make([]int, 0, len(errorsByTick))
	for tick := range errorsByTick {
		ticks = append(ticks, tick)
	}
	sort.Ints(ticks)
	
	for _, tick := range ticks {
		instances := errorsByTick[tick]
		unique := make(map[string]bool)
		for _, inst := range instances {
			unique[inst] = true
		}
		
		fmt.Printf("\nTick %d: %d unique instances not found\n", tick, len(unique))
		for inst := range unique {
			fmt.Printf("  • %s\n", inst)
		}
	}
}

func (a *LogAnalyzer) findTickForTimestamp(t time.Time) int {
	for tick, tickData := range a.TickMap {
		if t.After(tickData.Timestamp.Add(-time.Second)) &&
		   t.Before(tickData.Timestamp.Add(time.Second)) {
			return tick
		}
	}
	return -1
}