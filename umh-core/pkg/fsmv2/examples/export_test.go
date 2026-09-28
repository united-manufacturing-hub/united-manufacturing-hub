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

package examples

import "time"

// SetWaitForTimeoutForTest overrides how long a single WaitFor may take
// before it fails the run, and returns a func restoring the default.
func SetWaitForTimeoutForTest(d time.Duration) (restore func()) {
	prev := waitForTimeout
	waitForTimeout = d

	return func() {
		waitForTimeout = prev
	}
}
