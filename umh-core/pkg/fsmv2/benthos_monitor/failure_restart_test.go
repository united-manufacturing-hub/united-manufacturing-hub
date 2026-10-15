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

package fsmv2benthosmonitor

import (
	"strconv"
	"testing"
	"time"

	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// scrapes is one deps value and the verdict of its latest observe call.
type scrapes struct {
	t       *testing.T
	d       *benthosMonitorDeps
	verdict benthosmonitorservice.FailureVerdict
}

func newScrapes(t *testing.T) *scrapes {
	return &scrapes{t: t, d: &benthosMonitorDeps{}}
}

// at observes one scrape of the benthos at port with output error total n.
func (s *scrapes) at(at time.Time, port int, n int64) *scrapes {
	_, s.verdict = observe(s.d, port, outputErrors(n), at)

	return s
}

// wantErrorFree fails the test unless the latest verdict is error-free.
func (s *scrapes) wantErrorFree(when string) {
	s.t.Helper()
	if ok, reason := s.verdict.ErrorFree(); !ok {
		s.t.Fatalf("%s: ErrorFree() = (false, %q), want true", when, reason)
	}
}

// wantReason fails the test unless the latest verdict is not error-free with
// exactly this reason.
func (s *scrapes) wantReason(when, want string) {
	s.t.Helper()
	if ok, reason := s.verdict.ErrorFree(); ok || reason != want {
		s.t.Fatalf("%s: ErrorFree() = (%v, %q), want (false, %q)", when, ok, reason, want)
	}
}

// firedAt returns scrapes whose output_error went 0 -> n at failureT, so the
// signal fired at failureT.
func firedAt(t *testing.T, n int64) *scrapes {
	s := newScrapes(t).at(failureT.Add(-time.Second), testPort, 0).at(failureT, testPort, n)
	s.wantFailure("setup, at T", n)

	return s
}

// wantFailure fails the test unless the latest verdict names n output errors.
func (s *scrapes) wantFailure(when string, n int64) {
	s.t.Helper()
	label := "output errors"
	if n == 1 {
		label = "output error"
	}
	s.wantReason(when, "benthos reported "+strconv.FormatInt(n, 10)+" "+label+" in the last 10m0s")
}

// The config is fixed, benthos restarts, and its counter starts again at 0.
// The seed adds no false rise.
func TestObserveRestartToZeroClearsAtOnce(t *testing.T) {
	firedAt(t, 3).at(failureT.Add(time.Second), testPort, 0).wantErrorFree("first scrape after a restart to 0")
}

// Benthos restarts and counts 3 errors before the first scrape after the
// restart. The counter dropped from 7, yet the 3 count as new errors.
func TestObserveErrorsBeforeFirstScrapeAfterRestartCount(t *testing.T) {
	s := newScrapes(t).at(failureT.Add(-time.Second), testPort, 7).at(failureT, testPort, 7)
	s.wantErrorFree("before the restart, counter flat at 7")
	s.at(failureT.Add(time.Second), testPort, 3).wantFailure("first scrape after a restart, counter 3", 3)
}

// Benthos restarts and counts 3 errors before the first scrape after the
// restart, and that first scrape comes 15 minutes after the last one.
func TestObserveErrorsBeforeFirstScrapeAfterRestartCountAfterLongGap(t *testing.T) {
	s := newScrapes(t).at(failureT.Add(-time.Second), testPort, 7).at(failureT, testPort, 7)
	s.at(failureT.Add(15*time.Minute), testPort, 3).wantFailure("first scrape 15m later, after a restart, counter 3", 3)
}

// Scrapes fail for 15 minutes while benthos keeps counting errors.
func TestObserveRiseDuringLongOutageCounts(t *testing.T) {
	firedAt(t, 5).at(failureT.Add(15*time.Minute), testPort, 8).wantFailure("first scrape after a 15m outage, counter 5 -> 8", 3)
}

// Scrapes fail for 15 minutes while benthos counts no new error.
func TestObserveNoRiseDuringLongOutageReleases(t *testing.T) {
	firedAt(t, 5).at(failureT.Add(15*time.Minute), testPort, 5).wantErrorFree("first scrape after a 15m outage, counter still 5")
}

// Scrapes fail for just under the 10-minute window while benthos counts no new
// error. The window still holds the rise, so the signal stays fired. A rebuild
// on a shorter gap would release it.
func TestObserveGapShorterThanWindowKeepsSignal(t *testing.T) {
	firedAt(t, 5).at(failureT.Add(10*time.Minute-2*time.Second), testPort, 5).wantFailure("first scrape after a 9m58s outage, counter still 5", 5)
}

// MetricsPort changes without a worker restart, and the benthos on the new
// port reports 0.
func TestObservePortChangeIsARestart(t *testing.T) {
	firedAt(t, 3).at(failureT.Add(time.Second), testPort+1, 0).wantErrorFree("first scrape on a new port, counter 0")
}

// A scrape stamped at the same instant as the previous one shows a dropped
// counter; the next scrape, one second later, shows the same drop.
func TestObserveDropOnSameInstantScrapeWaitsForNextScrape(t *testing.T) {
	s := firedAt(t, 3)
	s.at(failureT, testPort, 0).wantFailure("same-instant scrape with a dropped counter", 3)
	s.at(failureT.Add(time.Second), testPort, 0).wantErrorFree("next scrape, counter still 0")
}
