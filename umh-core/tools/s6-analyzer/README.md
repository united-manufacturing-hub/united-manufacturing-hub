# S6 Analyzer

A diagnostic tool for analyzing S6 service status directories to help debug service issues in UMH Core.

## Purpose

The S6 Analyzer tool provides detailed insights into S6 service state by examining the status directory structure. This is particularly useful when debugging services that are stuck, failing to start, or behaving unexpectedly.

## Features

- **Service Status Analysis**: Reads and interprets the S6 status file format
- **Process Information**: Shows PID, PGID, uptime, and exit codes
- **Flag Interpretation**: Decodes S6 control flags (paused, finishing, want-up, ready)
- **Down File Detection**: Warns about presence of down files that prevent service startup
- **Run Script Inspection**: Shows permissions and first few lines of run scripts
- **Log Analysis**: Displays recent log entries from the service
- **Raw Status Debugging**: Shows hex dump of status files for detailed debugging

## Building

From the `tools/s6-analyzer` directory:

```bash
go build -o s6-analyzer main.go
```

## Usage

```bash
# Analyze a specific service
./s6-analyzer -service /data/services/redpanda-redpanda

# Alternative syntax
./s6-analyzer /data/services/redpanda-redpanda
```

## Output Example

```
=== Analyzing S6 Service: /data/services/redpanda-redpanda ===

[Using S6 Service Status() method]

Main Service Status:
  Status:       up
  PID:          1234
  PGID:         1234
  Uptime:       3600 seconds
  Ready time:   3595 seconds
  Flags:
    Paused:     false
    Finishing:  false
    Want up:    true
    Ready:      true
  Timestamps:
    Last changed: 2025-01-14 10:00:00 (1h ago)
    Last ready:   2025-01-14 10:00:05 (1h ago)

Run script:
  Path: /data/services/redpanda-redpanda/run
  Permissions: -rwxr-xr-x
  Size: 256 bytes
  First 5 lines:
    #!/usr/bin/execlineb -P
    foreground { redpanda start }
    ...
```

## Common Exit Codes

- **Exit 111**: Binary not found or not executable (common when the service executable is missing)
- **Exit 0**: Normal termination
- **Signal kills**: Service was terminated by a signal

## Integration with UMH Core

This tool uses the same S6 service package that UMH Core uses internally, ensuring consistent interpretation of status files. It's particularly useful for:

- Debugging FSM (Finite State Machine) issues
- Understanding why services fail to transition states
- Diagnosing startup failures
- Verifying S6 supervision tree health