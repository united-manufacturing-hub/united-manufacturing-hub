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
	"math/rand"
	"time"
)

const Int64Max = 1<<63 - 1

func GetBackoffTime(retries int64, slotTime time.Duration, maximum time.Duration) (backoff time.Duration) {

	defer func() {
		if r := recover(); r != nil {
			backoff = maximum
		}
	}()

	if slotTime <= 0 || retries <= 0 {
		return time.Duration(0)
	}
	// 2^retries - 1
	// -1 is omitted here, because the random function is [min, max)
	umax := uint64(uint64(1) << retries)
	if umax > Int64Max || umax == 0 {
		return maximum
	}
	max := int64(umax)
	/* #nosec G404 -- This doesn't need to by crypto secure */
	n := rand.Int63n(max)

	// Prevents overflow
	u64Time := uint64(slotTime.Nanoseconds()) * uint64(n)
	if u64Time > Int64Max {
		return maximum
	}

	backoff = time.Duration(n) * slotTime
	if backoff > maximum {
		backoff = maximum
	}
	return backoff
}

func SleepBackedOff(retries int64, slotTime time.Duration, maximum time.Duration) {
	time.Sleep(GetBackoffTime(retries, slotTime, maximum))
}
