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
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cactus/tai64"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
)

// buildFullServiceInfo converts S6StatusData into a complete ServiceInfo
// This adds the high-level business logic (down files, exit history, etc.)
// that's needed for the full Status() method but not for cleanup detection
func (s *DefaultService) buildFullServiceInfo(ctx context.Context, servicePath string, statusData *s6_shared.S6StatusData, fsService filesystem.Service) (s6_shared.ServiceInfo, error) {
	// Start with basic info from the binary status data
	info := s6_shared.ServiceInfo{
		Status:             s6_shared.ServiceUnknown,
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
		info.Status = s6_shared.ServiceUp
		// uptime is measured from the stamp timestamp
		info.Uptime = int64(now.Sub(statusData.StampTime).Seconds())
		info.ReadyTime = int64(now.Sub(statusData.ReadyTime).Seconds())
	} else {
		info.Status = s6_shared.ServiceDown
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
func (s *DefaultService) ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]s6_shared.ExitEvent, error) {
	// Build the full path to the dtally file.
	dtallyFile := filepath.Join(superviseDir, s6_shared.S6DtallyFileName)

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
	if len(data)%s6_shared.S6_DTALLY_PACK != 0 {
		// The file is considered corrupted or partially written.
		// In the C code, a truncation is attempted in this case.
		// Here, we return an error.
		return nil, fmt.Errorf("dtally file size (%d bytes) is not a multiple of record size (%d)", len(data), s6_shared.S6_DTALLY_PACK)
	}

	// Calculate the number of records.
	numRecords := len(data) / s6_shared.S6_DTALLY_PACK
	var history []s6_shared.ExitEvent

	// Process each dtally record.
	for i := 0; i < numRecords; i++ {
		offset := i * s6_shared.S6_DTALLY_PACK
		record := data[offset : offset+s6_shared.S6_DTALLY_PACK]

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

		history = append(history, s6_shared.ExitEvent{
			Timestamp: parsedTime,
			ExitCode:  exitCode,
			Signal:    signalNumber,
		})
	}

	return history, nil
}
