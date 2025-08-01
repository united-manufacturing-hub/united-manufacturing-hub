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
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cactus/tai64"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// S6StatusData holds the parsed binary data from S6's 43-byte status file
// This is the core parsed data that all other status operations build upon
//
// CRITICAL: This structure maps directly to S6's internal s6_svstatus_t structure
// Source: https://github.com/skarnet/s6/blob/main/src/supervision/s6-supervise.c
//
// THE RACE CONDITION PROBLEM:
// Integration tests were failing with "directory not empty" errors during service removal.
// Timeline analysis showed a 4.5ms race condition where our code attempted directory removal
// immediately after terminating S6 supervisors, but before S6 completed internal cleanup.
//
// THE SOLUTION - S6'S OWN CLEANUP SIGNALS:
// Instead of using arbitrary delays, we use S6's own internal state tracking via the
// IsFinishing flag. This flag is set/cleared by S6's internal state machine:
//
// 1. Service dies → uplastup_z() sets flagfinishing=1 (cleanup begins)
// 2. S6 runs finish script, cleans internal state, closes file handles
// 3. S6 calls set_down_and_ready() → flagfinishing=0 (cleanup complete)
// 4. NOW it's safe to remove directories
//
// Our isSupervisorCleanupComplete() waits for flagfinishing=0 before allowing removal,
// eliminating the race condition by using S6's own completion signals.
type S6StatusData struct {
	// Binary file data (timestamps, PID, flags, etc.)
	StampTime time.Time // When status last changed
	ReadyTime time.Time // When service was last ready
	Pid       int       // Process ID (0 = service is down)
	Pgid      int       // Process group ID
	Wstat     uint16    // Wait status for down services

	// S6 flags from the binary file
	// Source: s6-supervise.c static s6_svstatus_t status
	IsPaused    bool // Service is paused (status.flagpaused)
	IsFinishing bool // Service is shutting down (status.flagfinishing) - KEY for cleanup detection
	IsWantingUp bool // Service wants to be up (status.flagwantup)
	IsReady     bool // Service is ready (status.flagready)
}

// parseS6StatusFile reads and parses the 43-byte binary S6 status file
//
// This is the CENTRAL parsing function that all status operations use.
// It handles the low-level binary format parsing of S6's status file.
//
// WHY CENTRALIZED:
// - Eliminates code duplication between Status() and removal logic
// - Single source of truth for S6 binary format parsing
// - Easier to maintain and test
// - Consistent parsing across all use cases
//
// BINARY FORMAT VERIFICATION:
// This format was verified against S6 source code at:
// https://github.com/skarnet/s6/blob/main/src/supervision/s6-supervise.c
// The s6_svstatus_t structure and s6_svstatus_write() function confirm our parsing.
//
// CRITICAL FLAG MEANINGS (from S6 source):
// - flagfinishing=1: Set in uplastup_z() when service dies, cleanup begins
// - flagfinishing=0: Set in set_down_and_ready() when cleanup complete
// - This flag is THE authoritative signal for supervisor cleanup completion
//
// BINARY FORMAT (43 bytes total):
//
//	Bytes 0-11:  TAI64N timestamp when status last changed
//	Bytes 12-23: TAI64N timestamp when service was last ready
//	Bytes 24-31: Process ID (big-endian uint64)
//	Bytes 32-39: Process group ID (big-endian uint64)
//	Bytes 40-41: Wait status (big-endian uint16)
//	Byte 42:     Flags (bit 0=paused, bit 1=finishing, bit 2=want up, bit 3=ready)
func parseS6StatusFile(ctx context.Context, statusFilePath string, fsService filesystem.Service) (*S6StatusData, error) {
	// Read the 43-byte binary status file
	statusData, err := fsService.ReadFile(ctx, statusFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read status file: %w", err)
	}

	// Check if the status file has the expected size
	if len(statusData) != S6StatusFileSize {
		return nil, fmt.Errorf("invalid status file size: got %d bytes, expected %d", len(statusData), S6StatusFileSize)
	}

	// --- Parse the two TAI64N timestamps ---

	// Stamp: bytes [0:12] - When status last changed
	stampBytes := statusData[S6StatusChangedOffset : S6StatusChangedOffset+12]
	stampHex := "@" + hex.EncodeToString(stampBytes)
	stampTime, err := tai64.Parse(stampHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stamp (%s): %w", stampHex, err)
	}

	// Readystamp: bytes [12:24] - When service was last ready
	readyStampBytes := statusData[S6StatusReadyOffset : S6StatusReadyOffset+12]
	readyStampHex := "@" + hex.EncodeToString(readyStampBytes)
	readyTime, err := tai64.Parse(readyStampHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse readystamp (%s): %w", readyStampHex, err)
	}

	// --- Parse integer fields using big-endian encoding ---

	// PID: bytes [24:32] (8 bytes)
	pid := binary.BigEndian.Uint64(statusData[S6StatusPidOffset : S6StatusPidOffset+8])

	// PGID: bytes [32:40] (8 bytes)
	pgid := binary.BigEndian.Uint64(statusData[S6StatusPgidOffset : S6StatusPgidOffset+8])

	// Wait status: bytes [40:42] (2 bytes)
	wstat := binary.BigEndian.Uint16(statusData[S6StatusWstatOffset : S6StatusWstatOffset+2])

	// --- Parse flags (1 byte at offset 42) ---
	flags := statusData[S6StatusFlagsOffset]
	flagPaused := (flags & S6FlagPaused) != 0
	flagFinishing := (flags & S6FlagFinishing) != 0
	flagWantUp := (flags & S6FlagWantUp) != 0
	flagReady := (flags & S6FlagReady) != 0

	return &S6StatusData{
		StampTime:   stampTime,
		ReadyTime:   readyTime,
		Pid:         int(pid),
		Pgid:        int(pgid),
		Wstat:       wstat,
		IsPaused:    flagPaused,
		IsFinishing: flagFinishing,
		IsWantingUp: flagWantUp,
		IsReady:     flagReady,
	}, nil
}

// buildFullServiceInfo converts S6StatusData into a complete ServiceInfo
// This adds the high-level business logic (down files, exit history, etc.)
// that's needed for the full Status() method but not for cleanup detection
func (s *DefaultService) buildFullServiceInfo(ctx context.Context, servicePath string, statusData *S6StatusData, fsService filesystem.Service) (ServiceInfo, error) {
	// Start with basic info from the binary status data
	info := ServiceInfo{
		Status:             ServiceUnknown,
		Pid:                statusData.Pid,
		Pgid:               statusData.Pgid,
		IsPaused:           statusData.IsPaused,
		IsFinishing:        statusData.IsFinishing,
		IsWantingUp:        statusData.IsWantingUp,
		IsReady:            statusData.IsReady,
		LastChangedAt:      statusData.StampTime,
		LastReadyAt:        statusData.ReadyTime,
		LastDeploymentTime: getLastDeploymentTime(servicePath),
	}

	// --- Determine service status and calculate time fields ---
	now := time.Now().UTC()
	if statusData.Pid != 0 && !statusData.IsFinishing {
		info.Status = ServiceUp
		// uptime is measured from the stamp timestamp
		info.Uptime = int64(now.Sub(statusData.StampTime).Seconds())
		info.ReadyTime = int64(now.Sub(statusData.ReadyTime).Seconds())
	} else {
		info.Status = ServiceDown
		// Interpret wstat as a wait status
		ws := syscall.WaitStatus(statusData.Wstat)
		if ws.Exited() {
			info.ExitCode = ws.ExitStatus()
		} else if ws.Signaled() {
			// Record the signal number as a negative exit code
			info.ExitCode = -int(ws.Signal())
		} else {
			info.ExitCode = int(statusData.Wstat)
		}
		info.DownTime = int64(now.Sub(statusData.StampTime).Seconds())
		info.ReadyTime = int64(now.Sub(statusData.ReadyTime).Seconds())
	}

	// --- Add business logic fields (down files, exit history) ---

	// Determine if service is "wanted up": if no "down" file exists
	downFile := filepath.Join(servicePath, "down")
	downExists, _ := fsService.FileExists(ctx, downFile)
	info.WantUp = !downExists

	// Add exit history from the supervise directory
	superviseDir := filepath.Join(servicePath, "supervise")
	history, histErr := s.ExitHistory(ctx, superviseDir, fsService)
	if histErr == nil {
		info.ExitHistory = history
	} else {
		return info, fmt.Errorf("failed to get exit history: %w", histErr)
	}

	return info, nil
}

// ExitHistory retrieves the service exit history by reading the dtally file ("death_tally")
// directly from the supervise directory instead of invoking s6-svdt.
// The dtally file is a binary file containing a sequence of dtally records.
// Each record has the following structure:
//   - Bytes [0:12]: TAI64N timestamp (12 bytes)
//   - Byte 12:      Exit code (1 byte)
//   - Byte 13:      Signal number (1 byte)
//
// If the file size is not a multiple of the record size, it is considered corrupted.
// In that case, you may choose to truncate the file (as the C code does) or return an error.
func (s *DefaultService) ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]ExitEvent, error) {
	// Build the full path to the dtally file.
	dtallyFile := filepath.Join(superviseDir, S6DtallyFileName)

	// Check if the dtally file exists.
	exists, err := fsService.FileExists(ctx, dtallyFile)
	if err != nil {
		return nil, err
	}
	if !exists {
		// If the dtally file does not exist, no exit history is available.
		return nil, nil
	}

	// Read the entire dtally file.
	data, err := fsService.ReadFile(ctx, dtallyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read dtally file: %w", err)
	}
	if data == nil { // Empty history file
		return nil, nil
	}

	// Verify that the file size is a multiple of the dtally record size.
	if len(data)%S6_DTALLY_PACK != 0 {
		// The file is considered corrupted or partially written.
		// In the C code, a truncation is attempted in this case.
		// Here, we return an error.
		return nil, fmt.Errorf("dtally file size (%d bytes) is not a multiple of record size (%d)", len(data), S6_DTALLY_PACK)
	}

	// Calculate the number of records.
	numRecords := len(data) / S6_DTALLY_PACK
	var history []ExitEvent

	// Process each dtally record.
	for i := 0; i < numRecords; i++ {
		offset := i * S6_DTALLY_PACK
		record := data[offset : offset+S6_DTALLY_PACK]

		// Unpack the TAI64N timestamp (first 12 bytes) and convert it to a time.Time.
		// The timestamp is encoded as 12 bytes, which we first convert to a hex string,
		// then prepend "@" (as required by the tai64.Parse function) and parse.
		tai64Str := "@" + hex.EncodeToString(record[:12])
		parsedTime, err := tai64.Parse(tai64Str)
		if err != nil {
			// If parsing fails, skip this record.
			continue
		}

		// Unpack the exit code (13th byte).
		exitCode := int(record[12])
		signalNumber := int(record[13])

		history = append(history, ExitEvent{
			Timestamp: parsedTime,
			ExitCode:  exitCode,
			Signal:    signalNumber,
		})
	}

	return history, nil
}

// These constants define file locations and offsets for direct S6 supervision file access

const (
	// Source: https://github.com/skarnet/s6/blob/main/src/include/s6/supervise.h
	// S6SuperviseStatusFile is the status file in the supervise directory.
	S6SuperviseStatusFile = "status"

	// S6 status file format (43 bytes total):
	// Byte range | Description
	// -----------|------------
	// 0-11       | TAI64N timestamp when status last changed (12 bytes)
	// 12-23      | TAI64N timestamp when service was last ready (12 bytes)
	// 24-31      | Process ID (big-endian uint64, 8 bytes)
	// 32-39      | Process group ID (big-endian uint64, 8 bytes)
	// 40-41      | Wait status (big-endian uint16, 2 bytes)
	// 42         | Flags byte (1 byte: bit 0=paused, bit 1=finishing, bit 2=want up, bit 3=ready)

	// Source: https://github.com/skarnet/s6/blob/main/src/libs6/s6_svstatus_unpack.c

	// Offsets in the status file:
	S6StatusChangedOffset = 0  // TAI64N timestamp when status last changed (12 bytes)
	S6StatusReadyOffset   = 12 // TAI64N timestamp when service was last ready (12 bytes)
	S6StatusPidOffset     = 24 // Process ID (uint64, 8 bytes)
	S6StatusPgidOffset    = 32 // Process group ID (uint64, 8 bytes)
	S6StatusWstatOffset   = 40 // Wait status (uint16, 2 bytes)
	S6StatusFlagsOffset   = 42 // Flags byte (1 byte)

	// Flags in the flags byte:
	S6FlagPaused    = 0x01 // Service is paused
	S6FlagFinishing = 0x02 // Service is shutting down
	S6FlagWantUp    = 0x04 // Service wants to be up
	S6FlagReady     = 0x08 // Service is ready

	// Expected size of the status file:
	S6StatusFileSize = 43 // bytes
)

// Constants for dtally file processing.
// S6DtallyFileName is the filename for the death tally file.
// S6_DTALLY_PACK is the size of a single dtally record (TAI64N timestamp + exitcode + signal).
const (
	S6DtallyFileName = "death_tally"
	// As TAIN_PACK is 12 bytes, then each dtally record is 12 + 1 + 1 = 14 bytes.
	S6_DTALLY_PACK = 14
)
