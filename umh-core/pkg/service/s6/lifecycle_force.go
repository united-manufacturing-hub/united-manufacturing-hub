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
	"time"

	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// ForceCleanup performs aggressive cleanup for stuck services
// Uses comprehensive cleanup approach:
// - Process termination and supervisor killing
// - Comprehensive artifact removal.
func (s *DefaultService) ForceCleanup(ctx context.Context, artifacts *ServiceArtifacts, fsService filesystem.Service) error {
	if s == nil {
		return errors.New("lifecycle manager is nil")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if artifacts == nil {
		return errors.New("artifacts is nil")
	}

	s.logger.Warnf("Force cleaning service artifacts: service=%s, files=%d",
		filepath.Base(artifacts.ServiceDir), len(artifacts.CreatedFiles))

	// Create down files first to ensure services stay down
	if err := s.createDownFiles(ctx, artifacts, fsService); err != nil {
		s.logger.Warnf("Failed to create down files: %v", err)
		// Continue with cleanup even if down files fail
	}

	// Try the full skarnet sequence first as a best-effort approach
	s.logger.Debugf("Attempting skarnet sequence for force cleanup")

	// Step 1: Stop service cleanly
	if err := s.stopServiceCleanly(ctx, artifacts.ServiceDir, fsService); err != nil {
		s.logger.Warnf("Failed to stop service cleanly during force cleanup: %v", err)
		// Continue with cleanup even if stopping fails
	}

	// Step 2: Try to unsupervise service (may fail if service is corrupted)
	if err := s.unsuperviseService(ctx, artifacts.ServiceDir, fsService); err != nil {
		s.logger.Warnf("Failed to unsupervise service during force cleanup: %v", err)
		// Continue with direct directory removal if unsupervise fails
	}

	// Step 3: Remove directories with timeout awareness (fallback approach)
	if err := s.removeDirectoryWithTimeout(ctx, artifacts.ServiceDir, fsService); err != nil {
		s.logger.Warnf("Failed to remove service directory: %v", err)
	}

	if err := s.removeDirectoryWithTimeout(ctx, artifacts.LogDir, fsService); err != nil {
		s.logger.Warnf("Failed to remove log directory: %v", err)
	}

	// Step 4: Remove the repository directory. The scan dir (ServiceDir) is a
	// symlink; the actual service files live in RepositoryDir. Without this step
	// the repository is orphaned on disk and only recovered when the next Create
	// call detects the stale directory.
	if artifacts.RepositoryDir != "" {
		if err := s.removeDirectoryWithTimeout(ctx, artifacts.RepositoryDir, fsService); err != nil {
			s.logger.Warnf("Failed to remove repository directory: %v", err)
		}
	}

	// Step 5: tell s6-svscan to drop any stale supervisor entries for this
	// service. s6-svunlink normally notifies the scanner itself, but on
	// exit 111 it doesn't. Without this call, repeated cleanup cycles
	// accumulate stale entries until s6-svscan can no longer bring up new
	// services. See ENG-4862.
	//
	// Best-effort: a failure here is logged + counted via metric. Blocking
	// cleanup on a flaky scanner would be worse than the stale entries.
	if _, err := s.ExecuteS6Command(ctx, artifacts.ServiceDir, fsService,
		"s6-svscanctl", "-an", constants.S6BaseDir); err != nil {
		s.logger.Warnf("s6-svscanctl -an failed during force cleanup for %s: %v "+
			"(scanner may be out of sync — monitor .forceRemove.svscanctl_failed)",
			filepath.Base(artifacts.ServiceDir), err)
		metrics.IncErrorCount(metrics.ComponentS6Service,
			filepath.Base(artifacts.ServiceDir)+".forceRemove.svscanctl_failed")
	} else {
		s.logger.Infof("s6-svscanctl -an completed for %s (scanner synced after force cleanup)",
			filepath.Base(artifacts.ServiceDir))
	}

	// Verify cleanup completed. Propagate PathExists errors rather than
	// treating them as "not exists" — a false negative would cause ForceCleanup
	// to return nil while directories may still exist on disk.
	serviceExists, err := fsService.PathExists(ctx, artifacts.ServiceDir)
	if err != nil {
		return fmt.Errorf("failed to verify service cleanup: %w", err)
	}
	logExists, err := fsService.PathExists(ctx, artifacts.LogDir)
	if err != nil {
		return fmt.Errorf("failed to verify log cleanup: %w", err)
	}
	repoExists := false
	if artifacts.RepositoryDir != "" {
		repoExists, err = fsService.PathExists(ctx, artifacts.RepositoryDir)
		if err != nil {
			return fmt.Errorf("failed to verify repository cleanup: %w", err)
		}
	}

	if serviceExists || logExists || repoExists {
		return fmt.Errorf("force cleanup incomplete: service=%v, log=%v, repo=%v", serviceExists, logExists, repoExists)
	}

	s.logger.Infof("Force cleanup completed for service: %s", filepath.Base(artifacts.ServiceDir))

	return nil
}

// removeDirectoryWithTimeout removes a directory with timeout awareness
// this has a high context time and should ONLY be called during
// force cleanup operations where thoroughness is prioritized over speed.
func (s *DefaultService) removeDirectoryWithTimeout(ctx context.Context, path string, fsService filesystem.Service) error {
	// Log directory contents if not empty
	s.logDirectoryContentsIfNotEmpty(ctx, path, "directory", fsService)

	// Use a short timeout for chunk-based deletion
	// Use background context to ensure we get the full timeout regardless of outer context
	const chunkTimeout = 750 * time.Millisecond

	chunkCtx, cancel := context.WithTimeout(context.Background(), chunkTimeout)
	defer cancel()

	if err := fsService.RemoveAll(chunkCtx, path); err != nil {
		if errors.Is(chunkCtx.Err(), context.DeadlineExceeded) {
			s.logger.Debugf("Directory removal timed out for %s, will retry", path)

			return context.DeadlineExceeded
		}

		return err
	}

	return nil
}
