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
	"time"

	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_default"
)

type RedpandaFailure interface {
	// IsFailure checks if the log line contains a failure
	// transitionToRunningTime is the time when the service was transitioned to running
	// This can be used to ignore certain failures, which might occur during the startup phase
	IsFailure(log s6service.LogEntry, transitionToRunningTime time.Time) bool
}

// RedpandaFailures is a list of failure detectors (implements RedpandaFailure), each checking for a specific condition inside the log line.
var RedpandaFailures = []RedpandaFailure{
	&AddressAlreadyInUseFailure{},
	&ReactorStalledFailure{},
}

// AddressAlreadyInUseFailure is a failure that occurs when the address is already in use.
type AddressAlreadyInUseFailure struct{}

// IsFailure checks if the log line contains "Address already in use".
func (a *AddressAlreadyInUseFailure) IsFailure(log s6service.LogEntry, _ time.Time) bool {
	return strings.Contains(log.Content, "Address already in use")
}

// ReactorStalledFailure is a failure that occurs when the reactor is stalled.
type ReactorStalledFailure struct{}

// IsFailure checks if the log line contains "Reactor stalled for", and if so, if the number of milliseconds is greater than 500.
var reactorStallRegex = regexp.MustCompile(`Reactor stalled for (\d+) ms`)

func (r *ReactorStalledFailure) IsFailure(log s6service.LogEntry, transitionToRunningTime time.Time) bool {
	// Early return if the log line does not contain "Reactor stalled for"
	if !strings.Contains(log.Content, "Reactor stalled for") {
		return false
	}

	// If the stall is before the time that redpanda reported to be running, we can ignore it
	if log.Timestamp.Before(transitionToRunningTime) {
		return false
	}

	// Reactor stalls are always printed as milliseconds
	// https://github.com/scylladb/seastar/blob/e06b9092f921da1bcf7240f16adb9e6d2e227ae5/src/core/reactor.cc#L1456C17-L1456C28
	/*
		void cpu_stall_detector::generate_trace() {
		    auto delta = reactor::now() - _run_started_at;

		    _total_reported++;
		    if (_config.report) {
		        _config.report();
		        return;
		    }

		    backtrace_buffer buf;
		    buf.append("Reactor stalled for ");
		    buf.append_decimal(uint64_t(delta / 1ms));
		    buf.append(" ms");
		    print_with_backtrace(buf, _config.oneline);
		    maybe_report_kernel_trace(buf);
		}
	*/
	// Example line: Reactor stalled for 32 ms

	// Extract the number of milliseconds from the log line
	matches := reactorStallRegex.FindStringSubmatch(log.Content)
	if len(matches) < 2 {
		return false
	}

	// Convert the number of milliseconds to an integer
	ms, err := strconv.Atoi(matches[1])
	if err != nil {
		return false
	}

	// Return failure if the reactor stalled for more than 500ms
	// 500ms was chosen, as it is very unlikely to happen during normal operation
	return ms > 500
}
