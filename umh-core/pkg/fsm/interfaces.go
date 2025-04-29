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

package fsm

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// FSMActions defines the standard lifecycle actions that all FSM instances should implement
type FSMInstanceActions interface {
	// CreateInstance initiates the creation of a managed instance
	CreateInstance(ctx context.Context, filesystemService filesystem.Service) error

	// RemoveInstance initiates the removal of a managed instance
	RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error

	// StartInstance initiates the starting of a managed instance
	StartInstance(ctx context.Context, filesystemService filesystem.Service) error

	// StopInstance initiates the stopping of a managed instance
	StopInstance(ctx context.Context, filesystemService filesystem.Service) error

	// UpdateObservedStateOfInstance updates the observed state of the instance
	UpdateObservedStateOfInstance(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) error

	// CheckForCreation checks if the instance should be created
	CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool
}
