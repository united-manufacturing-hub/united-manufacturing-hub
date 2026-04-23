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

package example_child

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/snapshot"
)

// ExamplechildConfig re-exports snapshot.ExamplechildConfig so callers can use
// example_child.ExamplechildConfig without pulling in the snapshot leaf
// package directly. The underlying type lives in snapshot to allow the state
// sub-package (which the worker imports for init-only side effects) to
// reference Config/Status without creating an import cycle with the worker
// package.
type ExamplechildConfig = snapshot.ExamplechildConfig

// ExamplechildStatus re-exports snapshot.ExamplechildStatus for the same
// reason as ExamplechildConfig.
type ExamplechildStatus = snapshot.ExamplechildStatus
