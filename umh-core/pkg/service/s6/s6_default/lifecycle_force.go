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

package s6_default

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"errors"

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

	// Verify cleanup completed
	serviceExists, _ := fsService.PathExists(ctx, artifacts.ServiceDir)
	logExists, _ := fsService.PathExists(ctx, artifacts.LogDir)

	if serviceExists || logExists {
		return fmt.Errorf("force cleanup incomplete: service=%v, log=%v", serviceExists, logExists)
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
