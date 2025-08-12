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

package s6_shared

import (
	"sync"
)

// ServiceArtifacts represents the essential paths for an S6 service
// Tracks only essential root paths to minimize I/O operations and improve performance.
type ServiceArtifacts struct {
	// RemovalProgress tracks what has been completed during removal for idempotent incremental removal
	RemovalProgress *RemovalProgress
	// ServiceDir is the scan directory symlink path (e.g., /run/service/foo)
	ServiceDir string
	// RepositoryDir is the repository directory path (e.g., /data/services/foo)
	RepositoryDir string
	// LogDir is the external log directory (e.g., /data/logs/foo)
	LogDir string
	// TempDir is populated only during Create() for atomic operations (DEPRECATED: no longer used with symlink approach)
	TempDir string
	// CreatedFiles tracks all files created during service creation for health checks (paths point to repository files)
	CreatedFiles []string
	// RemovalProgressMutex secures concurrent access to RemovalProgress
	RemovalProgressMu sync.RWMutex
}

// RemovalProgress tracks the state of removal operations using the skarnet sequence
// Each field represents a step that has been completed and verified.
type RemovalProgress struct {
	// SymlinkRemoved indicates that the symlink has been removed from scan directory
	SymlinkRemoved bool
	// ServiceStopped indicates that s6-svc -wD -d has completed for both main and log services
	ServiceStopped bool
	// ServiceUnsupervised indicates that s6-svunlink has completed and supervisors have exited
	ServiceUnsupervised bool
	// DirectoriesRemoved indicates that both repository and log directories have been removed
	DirectoriesRemoved bool
}

// InitRemovalProgress initializes removal progress tracking if not already present.
func (artifacts *ServiceArtifacts) InitRemovalProgress() {
	artifacts.RemovalProgressMu.Lock()
	defer artifacts.RemovalProgressMu.Unlock()

	if artifacts.RemovalProgress == nil {
		artifacts.RemovalProgress = &RemovalProgress{}
	}
}

// IsFullyRemoved checks if all removal steps have been completed using the skarnet sequence.
func (artifacts *ServiceArtifacts) IsFullyRemoved() bool {
	artifacts.RemovalProgressMu.RLock()
	defer artifacts.RemovalProgressMu.RUnlock()

	if artifacts.RemovalProgress == nil {
		return false
	}

	p := artifacts.RemovalProgress

	return p.SymlinkRemoved && p.ServiceStopped && p.ServiceUnsupervised && p.DirectoriesRemoved
}
