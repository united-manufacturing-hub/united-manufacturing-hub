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

package s6

import (
	"fmt"
	"regexp"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
)

const (
	// DefaultLogArchiveCount is the number of archived log files to keep
	// for dynamically created services. We use 20 to provide adequate history
	// while limiting disk usage. Static services like umh-core use n5
	// (configured in s6-rc.d/umh-core-log/run) for lower retention.
	DefaultLogArchiveCount = 20
)

// runScriptTemplate is the template for the S6 run script.
const runScriptTemplate = `#!/command/execlineb -P

{{- range $key, $value := .Env }}
export {{ $key }} {{ $value }}
{{- end }}

{{- if .Env }}

{{- end }}
# Memory limit
# -m for total memory
{{- if ne .MemoryLimit 0 }}
s6-softlimit -m {{ .MemoryLimit }}
{{- end }}

# Wait for log service to be up before starting to prevent race condition
# Run s6-svwait as root since it needs access to supervise/control pipe
foreground { s6-svwait -u {{ .ServicePath }}/log }

# No privilege drop needed - service inherits non-root UID from container USER directive

# Keep stderr and stdout separate but both visible in logs
fdmove -c 2 1 
{{ range $index, $cmd := .Command }}{{ if eq $index 0 }}{{ $cmd }}{{ else }} {{ $cmd }}{{ end }}{{ end }}
`

// runScriptParser is a regexp to extract command from the run script
// This matches any text after "fdmove -c 2 1" on the same line or on a subsequent line.
var runScriptParser = regexp.MustCompile(`(?m)fdmove -c 2 1(?:\s+(.+)|$)`)

// envVarParser is a regexp to extract environment variables from the run script.
var envVarParser = regexp.MustCompile(`export\s+(\w+)\s+(.+)`)

// logFilesizeParser is a dedicated regexp to extract log filesize value from the log run script
// It matches the s6-log command format: "s6-log n20 s1024 T /path" and captures the filesize.
var logFilesizeParser = regexp.MustCompile(`s6-log\s+n\d+\s+s(\d+)\s+T\s+\S+`)

// memoryLimitParser is a regexp to extract the memory limit from the run script.
var memoryLimitParser = regexp.MustCompile(`s6-softlimit -m\s+(\d+)`)

func getLogRunScript(config s6serviceconfig.S6ServiceConfig, logDir string) (string, error) {
	// Build s6-log command directly, see https://skarnet.org/software/s6/s6-log.html
	// Note: We use s6-log directly instead of logutil-service because logutil-service
	// tries to drop privileges using s6-applyuidgid, which fails in non-root containers.
	// s6-log runs as the invoking user (umhuser) without requiring privilege changes.

	// ----------  ⚠️ Logging caveat (2025-07-04)  ----------
	//
	// We observed occasional binary blobs in `docker logs ...` that also
	// appeared in /data/logs/<svc>/current. After deep-dive investigation:
	//
	//   • s6-log writes only its own diagnostics to stderr; it never mirrors
	//     service output to stdout unless the `1` directive is present.
	//     Source: https://skarnet.org/software/s6/s6-log.html
	//   • The blobs leak during the few milliseconds before s6-log's pipe
	//     is ready (startup/rotation race in s6-svscan).
	//   • We still *need* those stderr diagnostics because they include
	//     fatal errors such as "unable to mkdir...", "disk full", and
	//     "broken pipe" alerts.
	//
	// Mitigation: fdmove -c 2 1 redirects stderr to stdout for logging.
	//
	// NOTE: Do **NOT** redirect to /dev/null — that would hide critical
	// s6-log warnings.
	//
	// --------------------------------------------------------

	// Build s6-log command with directives:
	// n{count} - number of archived files to keep
	// s{size} - rotate at this file size (only if LogFilesize > 0)
	// T - ISO 8601 timestamps (required for our time parser)
	// Important: This needs to be T (ISO 8601) as our time parser expects this format

	// Use our default archive count for dynamic services.
	// Static services like umh-core use n5 (configured in s6-rc.d/umh-core-log/run)
	// for lower retention since the main agent has more verbose logging.
	archiveCount := DefaultLogArchiveCount

	var s6LogCmd string
	if config.LogFilesize > 0 {
		// Include explicit size directive when LogFilesize is set
		s6LogCmd = fmt.Sprintf("s6-log n%d s%d T %s", archiveCount, config.LogFilesize, logDir)
	} else {
		// No size directive - s6-log uses its internal default
		s6LogCmd = fmt.Sprintf("s6-log n%d T %s", archiveCount, logDir)
	}

	// Create log run script
	logRunContent := fmt.Sprintf(`#!/command/execlineb -P
fdmove -c 2 1
foreground { mkdir -p %s }

%s`, logDir, s6LogCmd)

	return logRunContent, nil
}
