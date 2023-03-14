// Copyright 2023 UMH Systems GmbH
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

package internal

import (
	"testing"
	"time"
)

func Test_GetBackoffTime(t *testing.T) {
	for i := 0; i < 20; i++ {
		backOff := GetBackoffTime(int64(i), 1*time.Microsecond, 1*time.Second)
		t.Logf("Iteration %d: %s", i, backOff)
	}
}

func Test_CyclesUntilConverge(t *testing.T) {
	var testTimes = []time.Duration{
		time.Millisecond,
		time.Microsecond,
		time.Nanosecond,
	}
	for _, testTime := range testTimes {
		var i = int64(0)
		t.Logf("Testing %s", testTime)
		for {
			backOff := GetBackoffTime(int64(i), testTime, 1*time.Second)
			t.Logf("Iteration %d: %s", i, backOff)
			i += 1
			if backOff >= 1*time.Second {
				t.Logf("Converged after %d iterations", i)
				break
			}
		}
	}
}
