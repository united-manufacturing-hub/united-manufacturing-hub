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

// Health is the verdict for a poll that succeeded but found the target
// unhealthy: whether it is degraded and a human-readable reason.
//
// A worker can go degraded two ways: the poll itself errors, or the poll
// succeeds but the target is unhealthy. Both end up in the same degraded
// Status[T], but Health only decides the second. Poll errors become a degraded
// verdict directly, without calling Health (see simpleWorker.CollectObservedState).
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
