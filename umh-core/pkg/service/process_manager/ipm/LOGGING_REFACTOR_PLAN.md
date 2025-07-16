# IPM Hybrid Logging Refactor Plan

## Overview
Implement hybrid logging: capture stdout/stderr via pipes → store in memory buffer (10k lines) + write to files → rotate at 1MB, keep 10 files.

## New Components

### 1. `memory_log_buffer.go`
```go
type MemoryLogBuffer struct {
    mu      sync.RWMutex
    entries []process_shared.LogEntry
    maxSize int // 10k lines
}
```

### 2. `log_line_writer.go`
```go
type LogLineWriter struct {
    identifier   serviceIdentifier
    logManager   *LogManager
    memoryBuffer *MemoryLogBuffer
    currentFile  *os.File
    mutex        sync.Mutex
}
```

## File Changes

### `process_manager.go`
- **Add to struct**: `logBuffers map[serviceIdentifier]*MemoryLogBuffer`, `logWriters map[serviceIdentifier]*LogLineWriter`
- **GetLogs()**: Return from memory buffer instead of reading files
- **Close()**: Clean up log writers and goroutines

### `service_start.go`
- **startProcessAtomically()**: 
  - Remove shell redirection (`>> logfile 2>&1`)
  - Set `cmd.StdoutPipe()` and `cmd.StderrPipe()`
  - Start `readLogs()` goroutines for each pipe
  - Initialize memory buffer and log writer
- **Add readLogs()**: Scanner reads lines → create LogEntry → write to buffer+file

### `log_manager.go`
- **Add WriteLogLine()**: Write single line to current file + track size
- **Modify rotateLogFile()**: TAI64N rotation + cleanup old files (keep 10)
- **Add cleanupOldLogs()**: Sort by mtime, remove files beyond 10
- **RegisterService()**: Default 1MB threshold, track current file size

### `service_remove.go`
- **removeService()**: Close log writer, cleanup memory buffer
- **stopService()**: Close log writer (keep buffer since service exists)

### `loop.go`
- **step()**: Remove file-based log rotation (now per-line)

## Key Technical Changes

**Before:**
```go
// Shell redirection
redirectedCommand := fmt.Sprintf("%s >> %s 2>&1", commandPath, currentLogFile)
cmd = exec.Command("/bin/bash", "-c", redirectedCommand)
```

**After:**
```go
// Direct pipe capture
stdoutPipe, _ := cmd.StdoutPipe()
stderrPipe, _ := cmd.StderrPipe()
go readLogs(identifier, stdoutPipe, "stdout")
go readLogs(identifier, stderrPipe, "stderr")
```

**Before:**
```go
// Read from file
logContent, _ := fsService.ReadFile(ctx, currentLogFile)
return process_shared.ParseLogsFromBytes(logContent)
```

**After:**
```go
// Return from memory
return pm.logBuffers[identifier].GetEntries(), nil
```

## Benefits
- **Fast GetLogs()**: Memory access vs file I/O
- **Persistent files**: Still available for debugging
- **Bounded memory**: 10k lines max per service
- **Auto rotation**: 1MB → TAI64N files, keep 10
- **Real-time capture**: No polling delays 