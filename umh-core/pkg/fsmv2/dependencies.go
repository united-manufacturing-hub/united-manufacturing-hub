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

// TEMPORARY RE-EXPORTS - Remove in Phase 3
// This file re-exports types from deps/ for backward compatibility during migration.
package fsmv2

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"

type StateReader = deps.StateReader
type Dependencies = deps.Dependencies
type BaseDependencies = deps.BaseDependencies

// Identity is re-exported from deps because NewBaseDependencies uses it.
type Identity = deps.Identity

var NewBaseDependencies = deps.NewBaseDependencies
