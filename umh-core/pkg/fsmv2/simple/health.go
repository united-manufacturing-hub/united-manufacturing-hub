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

package simple

// Health is the verdict a worker's optional Health function returns for one
// poll: whether the polled target is degraded and a human-readable reason. The
// framework stamps it onto the Observation, where the fsmv2 state emits the
// reason and the fsmv1 adapter reads the verdict (ENG-5305, Q2/Q9).
type Health struct {
	// Reason is the human-readable explanation for the verdict, shown in logs
	// and the frontend (e.g. "port 502 unreachable").
	Reason string
	// Degraded is true when the polled target is unhealthy.
	Degraded bool
}

// Healthy returns a not-degraded verdict carrying reason.
func Healthy(reason string) Health {
	return Health{Degraded: false, Reason: reason}
}

// Degraded returns a degraded verdict carrying reason.
func Degraded(reason string) Health {
	return Health{Degraded: true, Reason: reason}
}
