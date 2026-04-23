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

package examplefailing

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
)

// ExamplefailingConfig re-exports snapshot.ExamplefailingConfig so callers can
// use examplefailing.ExamplefailingConfig without pulling in the snapshot leaf
// package directly. The underlying type lives in snapshot to allow the state
// sub-package (which the worker imports for init-only side effects) to
// reference Config/Status without creating an import cycle with the worker
// package.
type ExamplefailingConfig = snapshot.ExamplefailingConfig

// ExamplefailingStatus re-exports snapshot.ExamplefailingStatus for the same
// reason as ExamplefailingConfig.
type ExamplefailingStatus = snapshot.ExamplefailingStatus
