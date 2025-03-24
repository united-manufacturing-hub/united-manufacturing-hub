package s6

import "regexp"

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

// memoryLimitParser is a regexp to extract the memory limit from the run script
var memoryLimitParser = regexp.MustCompile(`s6-softlimit -m\s+(\d+)`)
