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

package s6_orig

import (
	"fmt"
	"regexp"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
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

# Drop privileges for the actual service
s6-setuidgid nobody 

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
// It specifically matches the pattern "export S6_LOGGING_SCRIPT "n20 s1024 T"" and captures the filesize.
var logFilesizeParser = regexp.MustCompile(`export\s+S6_LOGGING_SCRIPT\s+"n\d+\s+s(\d+)\s+T"`)

// memoryLimitParser is a regexp to extract the memory limit from the run script.
var memoryLimitParser = regexp.MustCompile(`s6-softlimit -m\s+(\d+)`)

func getLogRunScript(config s6serviceconfig.S6ServiceConfig, logDir string) (string, error) {
	// Create logutil-service command line, see also https://skarnet.org/software/s6/s6-log.html
	// logutil-service is a wrapper around s6_log and reads from the S6_LOGGING_SCRIPT environment variable
	// We overwrite the default S6_LOGGING_SCRIPT with our own if config.LogFilesize is set
	var logutilServiceCmd string

	logutilEnv := ""
	if config.LogFilesize > 0 {
		// n20 is currently hardcoded to match the default defined in the Dockerfile
		// using the same export method as in runScriptTemplate for env variables
		// Important: This needs to be T (ISO 8861) as our time parser expects this format
		logutilEnv = fmt.Sprintf("export S6_LOGGING_SCRIPT \"n%d s%d T\"", 20, config.LogFilesize)
	}
	// ----------  ⚠️ Logging caveat (2025-07-04)  ----------
	//
	// We observed occasional binary blobs in `docker logs ...` that also
	// appeared in /data/logs/<svc>/current. After deep-dive investigation:
	//
	//   • s6-log writes only its own diagnostics to stderr; it never mirrors
	//     service output to stdout unless the `1` directive is present.
	//     Source: https://skarnet.org/software/s6/s6-log.html
	//   • logutil-service passes exactly one action (the log directory), so
	//     duplicate output is *not* expected. The blobs leak during the
	//     few milliseconds before s6-log's pipe is ready (startup/rotation
	//     race in s6-svscan).
	//   • We still *need* those stderr diagnostics because they include
	//     fatal errors such as "unable to mkdir...", "disk full", and
	//     "broken pipe" alerts.
	//
	// Mitigation: quarantine everything the logger writes **to stderr**
	// into a side-channel file that does not pollute Docker logs but
	// remains available for post-mortem analysis.
	//
	// NOTE: Do **NOT** redirect to /dev/null — that would hide critical
	// s6-log warnings.
	//
	// --------------------------------------------------------

	// Keep stdout quiet (stops binary blob leakage), but leave stderr visible
	// in Docker logs for s6-log diagnostics
	logutilServiceCmd = fmt.Sprintf(
		"logutil-service %s 1>/dev/null",
		logDir,
	)

	// Create log run script
	logRunContent := fmt.Sprintf(`#!/command/execlineb -P
fdmove -c 2 1
foreground { mkdir -p %s }
foreground { chown -R nobody:nobody %s }

%s
%s`, logDir, logDir, logutilEnv, logutilServiceCmd)

	return logRunContent, nil
}
