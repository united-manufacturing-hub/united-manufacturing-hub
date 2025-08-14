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
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	timestampRegex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+)`)
	levelRegex     = regexp.MustCompile(`\[(DEBUG|INFO|WARN|ERROR)\]`)
	sourceRegex    = regexp.MustCompile(`\[([^\]]+)\]`)
	componentRegex = regexp.MustCompile(`\[([^\]]+)\]\s+(?:FSM|Reconciling|Setting desired state|Updated system snapshot|Starting Action|Action:)`)

	tickRegex        = regexp.MustCompile(`Updated system snapshot at tick (\d+)`)
	fsmAttemptRegex  = regexp.MustCompile(`FSM (\S+) attempting transition: current_state='([^']+)' -> event='([^']+)' \(desired_state='([^']+)'\)`)
	fsmSuccessRegex  = regexp.MustCompile(`FSM (\S+) successful transition: '([^']+)' -> '([^']+)' via event='([^']+)' \(desired_state='([^']+)'\)`)
	fsmFailedRegex   = regexp.MustCompile(`FSM (\S+) failed transition: current_state='([^']+)' -> event='([^']+)' \(desired_state='([^']+)'\)`)
	fsmDesiredRegex  = regexp.MustCompile(`Setting desired state of FSM (\S+) to (\S+)`)
	reconcileRegex   = regexp.MustCompile(`Reconciling (\S+) (?:manager )?at tick (\d+)`)
	actionStartRegex = regexp.MustCompile(`Starting Action: (.+)`)
	actionDoneRegex  = regexp.MustCompile(`Action: (.+) done`)
	errorRegex       = regexp.MustCompile(`Error (?:executing action|removing|creating|starting|stopping): (.+)`)
	startupRegex     = regexp.MustCompile(`Starting umh-core\.\.\.`)
)

func ParseLogFile(filename string) (*LogAnalyzer, error) {
	file, err := os.Open(filename) //nolint:gosec // G304: Log analyzer tool function, filename provided by user for legitimate log parsing
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	defer func() { _ = file.Close() }()

	analyzer := &LogAnalyzer{
		Entries:      make([]LogEntry, 0),
		TickMap:      make(map[int]*TickData),
		FSMHistories: make(map[string]*FSMHistory),
		Errors:       make([]LogEntry, 0),
		Starts:       make([]time.Time, 0),
		Sessions:     make([]SessionInfo, 0),
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.TrimSpace(line) == "" {
			continue
		}

		entry, err := parseLine(line)
		if err != nil {
			continue
		}

		analyzer.processEntry(entry)
		analyzer.Entries = append(analyzer.Entries, entry)

		// Check for startup
		if startupRegex.MatchString(line) {
			analyzer.Starts = append(analyzer.Starts, entry.Timestamp)
		}

		// Collect errors
		if entry.Level == "ERROR" || entry.EntryType == EntryTypeError {
			analyzer.Errors = append(analyzer.Errors, entry)
		}
	}

	err = scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	// Build sessions
	analyzer.buildSessions()

	return analyzer, nil
}

func parseLine(line string) (LogEntry, error) { //nolint:unparam // error return may be needed for future error handling
	entry := LogEntry{
		EntryType: EntryTypeUnknown,
	}

	if match := timestampRegex.FindStringSubmatch(line); match != nil {
		t, err := time.Parse("2006-01-02 15:04:05.000000000", match[1])
		if err == nil {
			entry.Timestamp = t
		}
	}

	if match := levelRegex.FindStringSubmatch(line); match != nil {
		entry.Level = match[1]
	}

	sourceMatches := sourceRegex.FindAllStringSubmatch(line, -1)
	if len(sourceMatches) >= 2 {
		entry.Source = sourceMatches[1][1]
	}

	if match := componentRegex.FindStringSubmatch(line); match != nil {
		entry.Component = match[1]
	}

	entry.Message = line

	if match := tickRegex.FindStringSubmatch(line); match != nil {
		entry.EntryType = EntryTypeTick
		entry.TickNumber, _ = strconv.Atoi(match[1])
	} else if match := fsmAttemptRegex.FindStringSubmatch(line); match != nil {
		entry.EntryType = EntryTypeFSMAttempt
		entry.FSMName = match[1]
		entry.FromState = match[2]
		entry.Event = match[3]
		entry.DesiredState = match[4]
	} else if match := fsmSuccessRegex.FindStringSubmatch(line); match != nil {
		entry.EntryType = EntryTypeFSMTransition
		entry.FSMName = match[1]
		entry.FromState = match[2]
		entry.ToState = match[3]
		entry.Event = match[4]
		entry.DesiredState = match[5]
	} else if match := fsmFailedRegex.FindStringSubmatch(line); match != nil {
		entry.EntryType = EntryTypeFSMFailed
		entry.FSMName = match[1]
		entry.FromState = match[2]
		entry.Event = match[3]
		entry.DesiredState = match[4]
	} else if match := fsmDesiredRegex.FindStringSubmatch(line); match != nil {
		entry.EntryType = EntryTypeFSMDesiredState
		entry.FSMName = match[1]
		entry.DesiredState = match[2]
	} else if match := reconcileRegex.FindStringSubmatch(line); match != nil {
		entry.EntryType = EntryTypeReconciliation
		entry.Component = match[1]
		entry.TickNumber, _ = strconv.Atoi(match[2])
	} else if match := actionStartRegex.FindStringSubmatch(line); match != nil {
		entry.EntryType = EntryTypeActionStart
		entry.Message = match[1]
	} else if match := actionDoneRegex.FindStringSubmatch(line); match != nil {
		entry.EntryType = EntryTypeActionDone
		entry.Message = match[1]
	} else if match := errorRegex.FindStringSubmatch(line); match != nil {
		entry.EntryType = EntryTypeError
		entry.Message = match[1]
	}

	return entry, nil
}

func (a *LogAnalyzer) processEntry(entry LogEntry) {
	switch entry.EntryType {
	case EntryTypeTick:
		a.CurrentTick = entry.TickNumber
		if _, exists := a.TickMap[entry.TickNumber]; !exists {
			a.TickMap[entry.TickNumber] = &TickData{
				Number:    entry.TickNumber,
				Timestamp: entry.Timestamp,
				Events:    make([]LogEntry, 0),
			}
		}

	case EntryTypeFSMTransition:
		a.recordFSMTransition(entry, true)

	case EntryTypeFSMAttempt:
		a.recordFSMTransition(entry, false)

	case EntryTypeFSMFailed:
		a.recordFSMTransition(entry, false)

	case EntryTypeUnknown, EntryTypeFSMDesiredState, EntryTypeReconciliation, EntryTypeActionStart, EntryTypeActionDone, EntryTypeError:
		// These entry types are recorded but don't require special processing
		// They will be added to the current tick's events during the add operation
	}

	if a.CurrentTick > 0 {
		if tick, exists := a.TickMap[a.CurrentTick]; exists {
			tick.Events = append(tick.Events, entry)
		}
	}
}

func (a *LogAnalyzer) recordFSMTransition(entry LogEntry, success bool) {
	if _, exists := a.FSMHistories[entry.FSMName]; !exists {
		a.FSMHistories[entry.FSMName] = &FSMHistory{
			Name:        entry.FSMName,
			Transitions: make([]FSMTransition, 0),
		}
	}

	transition := FSMTransition{
		Tick:         a.CurrentTick,
		Timestamp:    entry.Timestamp,
		FromState:    entry.FromState,
		ToState:      entry.ToState,
		Event:        entry.Event,
		DesiredState: entry.DesiredState,
		Success:      success,
	}

	a.FSMHistories[entry.FSMName].Transitions = append(
		a.FSMHistories[entry.FSMName].Transitions,
		transition,
	)
}

func (a *LogAnalyzer) buildSessions() {
	if len(a.Starts) == 0 {
		return
	}

	// Sort starts by time
	sort.Slice(a.Starts, func(i, j int) bool {
		return a.Starts[i].Before(a.Starts[j])
	})

	for i := range len(a.Starts) {
		session := SessionInfo{
			StartTime: a.Starts[i],
			StartTick: -1,
			EndTick:   -1,
		}

		// Find end time (next start or last entry)
		if i+1 < len(a.Starts) {
			session.EndTime = a.Starts[i+1]
		} else if len(a.Entries) > 0 {
			session.EndTime = a.Entries[len(a.Entries)-1].Timestamp
		}

		// Find tick range for this session
		for tick, tickData := range a.TickMap {
			if tickData.Timestamp.After(session.StartTime) &&
				(session.EndTime.IsZero() || tickData.Timestamp.Before(session.EndTime)) {
				if session.StartTick == -1 || tick < session.StartTick {
					session.StartTick = tick
				}

				if tick > session.EndTick {
					session.EndTick = tick
				}
			}
		}

		// Count errors in this session
		for _, err := range a.Errors {
			if err.Timestamp.After(session.StartTime) &&
				(session.EndTime.IsZero() || err.Timestamp.Before(session.EndTime)) {
				session.Errors++
			}
		}

		a.Sessions = append(a.Sessions, session)
	}
}
