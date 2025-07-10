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
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// ForceCleanup performs aggressive cleanup for stuck services
// Uses expert-recommended patterns:
// - Process termination and supervisor killing
// - Orphan process cleanup (optional)
// - Comprehensive artifact removal
func (s *DefaultService) ForceCleanup(ctx context.Context, artifacts *ServiceArtifacts, fsService filesystem.Service) error {
	if s == nil {
		return fmt.Errorf("lifecycle manager is nil")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if artifacts == nil {
		return fmt.Errorf("artifacts is nil")
	}

	s.logger.Warnf("Force cleaning service artifacts: %+v", artifacts)

	// Create down files first
	if err := s.createDownFiles(ctx, artifacts, fsService); err != nil {
		s.logger.Warnf("Failed to create down files: %v", err)
		// Continue with cleanup even if down files fail
	}

	// Best-effort process termination
	if err := s.terminateProcesses(ctx, artifacts, fsService); err != nil {
		s.logger.Warnf("Failed to terminate processes: %v", err)
		// Continue with cleanup even if process termination fails
	}

	// Kill supervise processes
	if err := s.killSupervisors(ctx, artifacts, fsService); err != nil {
		s.logger.Warnf("Failed to kill supervisor processes: %v", err)
		// Continue with cleanup even if supervisor killing fails
	}

	// TODO: Consider implementing orphan process cleanup in the future
	// Previous implementation had goroutine leak - spawned background goroutine that did nothing
	// If needed, implement with proper lifecycle management and actual cleanup logic

	// Remove directories with timeout awareness
	if err := s.removeDirectoryWithTimeout(ctx, artifacts.ServiceDir, fsService); err != nil {
		s.logger.Warnf("Failed to remove service directory: %v", err)
	}

	if err := s.removeDirectoryWithTimeout(ctx, artifacts.LogDir, fsService); err != nil {
		s.logger.Warnf("Failed to remove log directory: %v", err)
	}

	// Verify cleanup completed
	serviceExists, _ := fsService.PathExists(ctx, artifacts.ServiceDir)
	logExists, _ := fsService.PathExists(ctx, artifacts.LogDir)

	if serviceExists || logExists {
		return fmt.Errorf("force cleanup incomplete: service=%v, log=%v", serviceExists, logExists)
	}

	s.logger.Infof("Force cleanup completed for service artifacts: %+v", artifacts)
	return nil
}

// killSupervisors kills s6-supervise processes
func (s *DefaultService) killSupervisors(ctx context.Context, artifacts *ServiceArtifacts, fsService filesystem.Service) error {
	supervisePaths := []string{
		filepath.Join(artifacts.ServiceDir, "supervise"),
		filepath.Join(artifacts.ServiceDir, "log", "supervise"),
	}

	var lastErr error
	for _, supervisePath := range supervisePaths {
		pidFile := filepath.Join(supervisePath, "pid")
		if data, err := fsService.ReadFile(ctx, pidFile); err == nil && len(data) > 0 {
			if pidStr := strings.TrimSpace(string(data)); pidStr != "" {
				// Try SIGTERM first
				if _, err := fsService.ExecuteCommand(ctx, "kill", "-TERM", pidStr); err != nil {
					s.logger.Debugf("Failed to kill supervisor process %s: %v", pidStr, err)
					lastErr = err
				}
			}
		}
	}

	return lastErr
}

// removeDirectoryWithTimeout removes a directory with timeout awareness
// this has a high context time and should ONLY be called during
// force cleanup operations where thoroughness is prioritized over speed.
func (s *DefaultService) removeDirectoryWithTimeout(ctx context.Context, path string, fsService filesystem.Service) error {
	// Use a short timeout for chunk-based deletion
	const chunkTimeout = 750 * time.Millisecond

	chunkCtx, cancel := context.WithTimeout(ctx, chunkTimeout)
	defer cancel()

	if err := fsService.RemoveAll(chunkCtx, path); err != nil {
		if chunkCtx.Err() == context.DeadlineExceeded {
			s.logger.Debugf("Directory removal timed out for %s, will retry", path)
			return context.DeadlineExceeded
		}
		return err
	}

	return nil
}
