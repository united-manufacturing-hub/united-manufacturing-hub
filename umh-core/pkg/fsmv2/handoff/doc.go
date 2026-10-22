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

// Package handoff defines the message type for handing a received message from
// an FSMv2 worker to the pkg/communicator router: the received message bytes,
// the sender the message names, and a trace ID. The package is temporary. It
// is removed under ENG-7166 once the communicator no longer needs it.
package handoff
