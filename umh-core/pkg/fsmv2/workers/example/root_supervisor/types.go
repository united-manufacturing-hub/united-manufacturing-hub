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

// Package root_supervisor demonstrates how to use the generic root package
// with custom child worker types. This example shows the passthrough pattern
// where the root supervisor dynamically creates children based on YAML config.
package root_supervisor

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/root_supervisor/snapshot"
)

// Re-export child types from snapshot package for convenience.
// Root types are not needed - we use the generic root package instead.
type (
	ChildSnapshot      = snapshot.ChildSnapshot
	ChildDesiredState  = snapshot.ChildDesiredState
	ChildObservedState = snapshot.ChildObservedState
)
