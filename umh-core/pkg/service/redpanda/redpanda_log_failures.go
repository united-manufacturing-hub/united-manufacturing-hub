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

package redpanda

import (
	"regexp"
	"strconv"
	"strings"
)

type RedpandaFailure interface {
	IsFailure(logLine string) bool
}

var RedpandaFailures = []RedpandaFailure{
	&AddressAlreadyInUseFailure{},
	&ReactorStalledFailure{},
}

type AddressAlreadyInUseFailure struct{}

func (a *AddressAlreadyInUseFailure) IsFailure(logLine string) bool {
	return strings.Contains(logLine, "Address already in use")
}

type ReactorStalledFailure struct{}

var reactorStallRegex = regexp.MustCompile(`Reactor stalled for (\d+) ms`)

func (r *ReactorStalledFailure) IsFailure(logLine string) bool {
	// Early return if the log line does not contain "Reactor stalled for"
	if !strings.Contains(logLine, "Reactor stalled for") {
		return false
	}

	// Reactor stalls are always printed as milliseconds
	// https://github.com/scylladb/seastar/blob/e06b9092f921da1bcf7240f16adb9e6d2e227ae5/src/core/reactor.cc#L1456C17-L1456C28
	// Example line: Reactor stalled for 32 ms

	// Extract the number of milliseconds from the log line
	matches := reactorStallRegex.FindStringSubmatch(logLine)
	if len(matches) < 2 {
		return false
	}

	// Convert the number of milliseconds to an integer
	ms, err := strconv.Atoi(matches[1])
	if err != nil {
		return false
	}

	// Return failure if the reactor stalled for more than 500ms
	return ms > 500
}
