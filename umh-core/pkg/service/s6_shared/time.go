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

package s6_shared

import (
	"errors"
	"time"
)

// BenchmarkParseNano-24                          62064972              18.59 ns/op            0 B/op          0 allocs/op
// BenchmarkTimeParseStandard-24                  8403676               139.8 ns/op            0 B/op          0 allocs/op

// ParseNano returns a UTC time.Time.
// The input must be exactly "YYYY-MM-DD HH:MM:SS.NNNNNNNNN".
func ParseNano(s string) (time.Time, error) { //nolint:varnamelen // Performance-critical parsing function - standard string param name
	if len(s) != 29 {
		return time.Time{}, errors.New("invalid length")
	}

	// quick delimiter sanity
	if s[4] != '-' || s[7] != '-' || s[10] != ' ' ||
		s[13] != ':' || s[16] != ':' || s[19] != '.' {
		return time.Time{}, errors.New("invalid delimiters")
	}

	// fast ASCII → int
	toInt := func(b byte, c byte) int { return int(b-'0')*10 + int(c-'0') }

	year := (int(s[0]-'0')*1000 +
		int(s[1]-'0')*100 +
		int(s[2]-'0')*10 +
		int(s[3]-'0'))

	month := toInt(s[5], s[6])
	day := toInt(s[8], s[9])
	hour := toInt(s[11], s[12])
	minute := toInt(s[14], s[15])
	sec := toInt(s[17], s[18])

	// 9-digit nanoseconds: positions 20–28 are guaranteed digits by the s6 format and length check
	nano := 0
	for i := 20; i < 29; i++ {
		nano = nano*10 + int(s[i]-'0')
	}

	return time.Date(year, time.Month(month), day, hour, minute, sec, nano, time.UTC), nil
}
