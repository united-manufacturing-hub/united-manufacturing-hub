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

// Package state provides FSM state implementations for the example parent worker.
//
// # Documentation
//
// This package is referenced by pkg/fsmv2/doc.go as an example of:
//   - Parent-child hierarchical composition
//   - State coordination via ChildStartStates
//
// When modifying these files, verify doc.go references remain accurate.
//
// See workers/example/doc.go for the full example worker documentation.
package state
