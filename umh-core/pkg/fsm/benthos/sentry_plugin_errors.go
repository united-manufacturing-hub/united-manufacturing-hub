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

package benthos

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// pluginErrorReportingEnabled lists the benthos-umh plugin IDs (as returned
// by dataflowcomponentserviceconfig.BenthosPluginID) reported to Sentry.
var pluginErrorReportingEnabled = map[string]bool{
	"ads": true,
}

const pluginErrorDebounceWindow = 1 * time.Hour

// pluginErrorLogWindowMaxLines caps attached log lines against chatty bursts.
const pluginErrorLogWindowMaxLines = 500

// pluginErrorEvent carries the data needed to report a plugin runtime error
// to Sentry off the hot reconcile loop.
type pluginErrorEvent struct {
	protocol       string
	instanceID     string
	triggerMessage string
	recentLogs     []s6service.LogEntry
	logger         *zap.SugaredLogger
}

// Building a Sentry event captures a stack trace and, at error level, dumps
// every goroutine in the process, so it stays off the reconcile goroutine.
// 64 is a generous headroom over realistic concurrent bridge counts per
// instance (~5 bridges/CPU core, see agent.enableResourceLimitBlocking); not
// derived from a hard requirement.
var pluginErrorCh = make(chan pluginErrorEvent, 64)

var (
	pluginErrorLastSent   = make(map[string]time.Time)
	pluginErrorLastSentMu sync.Mutex
)

func init() {
	go processPluginErrors()
}

// processPluginErrors turns queued plugin error events into Sentry reports.
// Debouncing already happened before the event was queued (see
// reportPluginErrorAsync), so every event here gets sent.
func processPluginErrors() {
	for ev := range pluginErrorCh {
		sentry.ReportIssueWithContext(
			pluginRuntimeError(),
			sentry.IssueTypeError,
			ev.logger,
			map[string]interface{}{
				// Fingerprint hint so Sentry groups per protocol (message stays fixed).
				"operation":    fmt.Sprintf("%s_plugin_runtime_error", ev.protocol),
				"service_type": "benthos",
				"protocol":     ev.protocol,
				"instance_id":  ev.instanceID,
				// []string, not string: routes to Sentry's untruncated "extra" context.
				"log_message": []string{ev.triggerMessage},
				"recent_logs": formatLogEntries(ev.recentLogs),
			},
		)
	}
}

// shouldReportPluginError debounces per protocol+instance, called from the
// reconcile goroutine before any log scanning so a persistent error only
// pays that cost once per pluginErrorDebounceWindow, not every tick.
func shouldReportPluginError(protocol, instanceID string) bool {
	pluginErrorLastSentMu.Lock()
	defer pluginErrorLastSentMu.Unlock()

	if !pluginErrorReportingEnabled[protocol] {
		return false
	}

	key := protocol + "|" + instanceID

	if last, ok := pluginErrorLastSent[key]; ok && time.Since(last) < pluginErrorDebounceWindow {
		return false
	}

	pluginErrorLastSent[key] = time.Now()

	return true
}

// reportPluginErrorAsync queues a plugin runtime error for Sentry reporting,
// no-op if protocol isn't enabled or is still debounced (checked before any
// log scanning). Non-blocking: safe to call from the reconcile loop.
// logs/currentTime/logWindow are the same window already used for degraded
// detection.
func reportPluginErrorAsync(protocol, instanceID string, logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration, triggerMessage string, logger *zap.SugaredLogger) {
	if !shouldReportPluginError(protocol, instanceID) {
		return
	}

	event := pluginErrorEvent{
		protocol:       protocol,
		instanceID:     instanceID,
		triggerMessage: triggerMessage,
		recentLogs:     recentLogsWithinWindow(logs, currentTime, logWindow, pluginErrorLogWindowMaxLines),
		logger:         logger,
	}

	select {
	case pluginErrorCh <- event:
	default: // drop rather than block reconcile
	}
}

// recentLogsWithinWindow returns the entries of logs newer than
// currentTime-window, capped to the last maxLines entries.
func recentLogsWithinWindow(logs []s6service.LogEntry, currentTime time.Time, window time.Duration, maxLines int) []s6service.LogEntry {
	cutoff := currentTime.Add(-window)

	var recent []s6service.LogEntry

	for _, l := range logs {
		if l.Timestamp.Before(cutoff) {
			continue
		}

		recent = append(recent, l)
	}

	if len(recent) > maxLines {
		recent = recent[len(recent)-maxLines:]
	}

	return recent
}

// formatLogEntries renders log entries as "<timestamp>  <content>" lines.
func formatLogEntries(logs []s6service.LogEntry) []string {
	lines := make([]string, 0, len(logs))

	for _, l := range logs {
		lines = append(lines, l.Timestamp.UTC().Format(time.RFC3339Nano)+"  "+l.Content)
	}

	return lines
}

// Fixed message: Sentry fingerprints on it, so no interpolation (see
// s6DirectoryHealthError for the same convention).
func pluginRuntimeError() error {
	return errors.New("Benthos plugin runtime error detected in logs")
}
