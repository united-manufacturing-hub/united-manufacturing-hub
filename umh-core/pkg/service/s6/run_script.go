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

// runScriptTemplate is the template for the S6 run script
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

# Drop privileges
s6-setuidgid nobody 

# Keep stderr and stdout separate but both visible in logs
fdmove -c 2 1 
{{ range $index, $cmd := .Command }}{{ if eq $index 0 }}{{ $cmd }}{{ else }} {{ $cmd }}{{ end }}{{ end }}
`

// runScriptParser is a regexp to extract command from the run script
// This matches any text after "fdmove -c 2 1" on the same line or on a subsequent line
var runScriptParser = regexp.MustCompile(`(?m)fdmove -c 2 1(?:\s+(.+)|$)`)

// envVarParser is a regexp to extract environment variables from the run script
var envVarParser = regexp.MustCompile(`export\s+(\w+)\s+(.+)`)

// logFilesizeParser is a dedicated regexp to extract log filesize value from the log run script
// It specifically matches the pattern "export S6_LOGGING_SCRIPT "n20 s1024 T"" and captures the filesize
var logFilesizeParser = regexp.MustCompile(`export\s+S6_LOGGING_SCRIPT\s+"n\d+\s+s(\d+)\s+T"`)

// memoryLimitParser is a regexp to extract the memory limit from the run script
var memoryLimitParser = regexp.MustCompile(`s6-softlimit -m\s+(\d+)`)

func getLogRunScript(config s6serviceconfig.S6ServiceConfig, logDir string) (string, error) {
	// Create logutil-service command line, see also https://skarnet.org/software/s6/s6-log.html
	// logutil-service is a wrapper around s6_log and reads from the S6_LOGGING_SCRIPT environment variable
	// We overwrite the default S6_LOGGING_SCRIPT with our own if config.LogFilesize is set
	logutilServiceCmd := ""
	logutilEnv := ""
	if config.LogFilesize > 0 {
		// n20 is currently hardcoded to match the default defined in the Dockerfile
		// using the same export method as in runScriptTemplate for env variables
		// Important: This needs to be T (ISO 8861) as our time parser expects this format
		logutilEnv = fmt.Sprintf("export S6_LOGGING_SCRIPT \"n%d s%d T\"", 20, config.LogFilesize)
	}
	logutilServiceCmd = fmt.Sprintf("logutil-service %s", logDir)

	// Create log run script
	logRunContent := fmt.Sprintf(`#!/command/execlineb -P
fdmove -c 2 1
foreground { mkdir -p %s }
foreground { chown -R nobody:nobody %s }
%s
%s
`, logDir, logDir, logutilEnv, logutilServiceCmd)

	return logRunContent, nil
}
