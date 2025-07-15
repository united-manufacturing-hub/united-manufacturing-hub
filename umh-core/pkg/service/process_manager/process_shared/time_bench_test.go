// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package process_shared

import (
	"testing"
	"time"
)

var (
	testTimestamp = "2023-05-15 12:34:56.123456789"
	timeLayout    = "2006-01-02 15:04:05.999999999"
)

func BenchmarkParseNano(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseNano(testTimestamp)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTimeParseStandard(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := time.Parse(timeLayout, testTimestamp)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Test for result equality to verify both functions produce the same result
func TestParseEquality(t *testing.T) {
	nano, err := ParseNano(testTimestamp)
	if err != nil {
		t.Fatal(err)
	}

	standard, err := time.Parse(timeLayout, testTimestamp)
	if err != nil {
		t.Fatal(err)
	}

	if !nano.Equal(standard) {
		t.Fatalf("Time mismatch:\nParseNano:    %v\ntime.Parse:   %v", nano, standard)
	}
}
