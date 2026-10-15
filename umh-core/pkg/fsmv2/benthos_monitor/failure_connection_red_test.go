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
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// conn is one path's connection counters.
type conn struct{ up, lost, failed int64 }

// connScrape returns a scrape with the given input and output paths. Every
// other counter is fixed, so only the connection counters change between
// scrapes.
func connScrape(inputs, outputs map[string]conn) benthosmonitorservice.Metrics {
	m := benthosmonitorservice.Metrics{
		Inputs:  map[string]benthosmonitorservice.InputInstance{},
		Outputs: map[string]benthosmonitorservice.OutputInstance{},
	}
	for path, c := range inputs {
		m.Inputs[path] = benthosmonitorservice.InputInstance{Received: 10, ConnectionUp: c.up, ConnectionLost: c.lost, ConnectionFailed: c.failed}
	}
	for path, c := range outputs {
		m.Outputs[path] = benthosmonitorservice.OutputInstance{Sent: 10, ConnectionUp: c.up, ConnectionLost: c.lost, ConnectionFailed: c.failed}
	}

	return m
}

// connSteps observes one scrape per second from failureT and returns each
// scrape's ErrorFree result.
func connSteps(scrapes ...benthosmonitorservice.Metrics) ([]bool, []string) {
	oks := make([]bool, 0, len(scrapes))
	reasons := make([]string, 0, len(scrapes))
	d := &benthosMonitorDeps{}
	for i, m := range scrapes {
		_, v := observe(d, testPort, m, failureT.Add(time.Duration(i)*time.Second))
		ok, reason := v.ErrorFree()
		oks = append(oks, ok)
		reasons = append(reasons, reason)
	}

	return oks, reasons
}

func in(c conn) map[string]conn { return map[string]conn{"root.input": c} }

// TestObserveFailedConnectFiresAtOnce: an input that never connected and
// failed to connect is not connected, from the first scrape that shows it.
func TestObserveFailedConnectFiresAtOnce(t *testing.T) {
	oks, reasons := connSteps(connScrape(in(conn{}), nil), connScrape(in(conn{failed: 3}), nil))
	want := "benthos reports 1 input not connected"
	if oks[1] || reasons[1] != want {
		t.Fatalf("input failed 3 times, never up: ErrorFree() = (%v, %q), want (false, %q)", oks[1], reasons[1], want)
	}
}

// TestObserveReconnectReleasesWithinTwoScrapes: once benthos reconnects, the
// verdict is error-free within two scrapes, not one failure window later.
func TestObserveReconnectReleasesWithinTwoScrapes(t *testing.T) {
	down := connScrape(in(conn{failed: 3}), nil)
	up := connScrape(in(conn{up: 1, failed: 3}), nil)
	oks, reasons := connSteps(connScrape(in(conn{}), nil), down, down, up, up)
	if oks[2] {
		t.Fatalf("setup: still down on the third scrape, but ErrorFree() = true")
	}
	if !oks[4] {
		t.Fatalf("two scrapes after the reconnect: ErrorFree() = (false, %q), want true", reasons[4])
	}
}

// TestObserveLostConnectionFiresAndReconnectReleases: a connection that was up
// and was lost is not connected until connection_up rises again.
func TestObserveLostConnectionFiresAndReconnectReleases(t *testing.T) {
	oks, reasons := connSteps(
		connScrape(in(conn{up: 1}), nil),
		connScrape(in(conn{up: 1, lost: 1}), nil),
		connScrape(in(conn{up: 2, lost: 1}), nil),
		connScrape(in(conn{up: 2, lost: 1}), nil),
	)
	if !oks[0] {
		t.Fatalf("connected input: ErrorFree() = (false, %q), want true", reasons[0])
	}
	if want := "benthos reports 1 input not connected"; oks[1] || reasons[1] != want {
		t.Fatalf("input lost: ErrorFree() = (%v, %q), want (false, %q)", oks[1], reasons[1], want)
	}
	if !oks[3] {
		t.Fatalf("after the reconnect: ErrorFree() = (false, %q), want true", reasons[3])
	}
}

// TestObserveSlowFirstConnectIsNotDown: an input still making its first
// connect, with no failed attempt yet, is not reported.
func TestObserveSlowFirstConnectIsNotDown(t *testing.T) {
	startup := connScrape(in(conn{}), nil)
	oks, reasons := connSteps(startup, startup, startup, startup)
	for i, ok := range oks {
		if !ok {
			t.Fatalf("scrape %d of a first connect in progress: ErrorFree() = (false, %q), want true", i, reasons[i])
		}
	}
}

// TestObserveConnectionJudgedPerPath: one connected path does not hide another
// that is down. Summed across both paths, up exceeds lost.
func TestObserveConnectionJudgedPerPath(t *testing.T) {
	inputs := map[string]conn{"root.input.broker.0": {up: 1}, "root.input.broker.1": {failed: 2}}
	oks, reasons := connSteps(connScrape(map[string]conn{"root.input.broker.0": {up: 1}, "root.input.broker.1": {}}, nil), connScrape(inputs, nil))
	if want := "benthos reports 1 input not connected"; oks[1] || reasons[1] != want {
		t.Fatalf("one of two input paths down: ErrorFree() = (%v, %q), want (false, %q)", oks[1], reasons[1], want)
	}
}

// TestObserveOutputConnection: the output side follows the same rule and is
// named as an output.
func TestObserveOutputConnection(t *testing.T) {
	out := func(c conn) map[string]conn { return map[string]conn{"root.output": c} }
	oks, reasons := connSteps(connScrape(nil, out(conn{up: 1})), connScrape(nil, out(conn{up: 1, lost: 1})))
	if want := "benthos reports 1 output not connected"; oks[1] || reasons[1] != want {
		t.Fatalf("output lost: ErrorFree() = (%v, %q), want (false, %q)", oks[1], reasons[1], want)
	}
}

// TestObserveTwoPathsDownIsPlural: the reason counts the paths that are down.
func TestObserveTwoPathsDownIsPlural(t *testing.T) {
	start := map[string]conn{"root.input.broker.0": {}, "root.input.broker.1": {}}
	down := map[string]conn{"root.input.broker.0": {failed: 1}, "root.input.broker.1": {failed: 1}}
	oks, reasons := connSteps(connScrape(start, nil), connScrape(down, nil))
	if want := "benthos reports 2 inputs not connected"; oks[1] || reasons[1] != want {
		t.Fatalf("two input paths down: ErrorFree() = (%v, %q), want (false, %q)", oks[1], reasons[1], want)
	}
}

// TestObserveErrorOutranksConnection: an output error (tier 0) is named before
// an input that is not connected (tier 1).
func TestObserveErrorOutranksConnection(t *testing.T) {
	d := &benthosMonitorDeps{}
	first := connScrape(in(conn{}), nil)
	observe(d, testPort, first, failureT.Add(-time.Second))
	second := connScrape(in(conn{failed: 2}), nil)
	second.Outputs["root.output"] = benthosmonitorservice.OutputInstance{Sent: 10, Error: 1}
	_, v := observe(d, testPort, second, failureT)
	want := "benthos reported 1 output error in the last 10m0s"
	if ok, reason := v.ErrorFree(); ok || reason != want {
		t.Fatalf("output error and input down: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
	if n := len(v.Fired); n != 2 {
		t.Errorf("fired entries = %d, want 2", n)
	}
}

// TestPollJudgesConnectionFromTheWire: the connection counters, read from a
// real /metrics body, reach the status verdict. This covers the parser's names
// for connection_up and connection_failed.
func TestPollJudgesConnectionFromTheWire(t *testing.T) {
	var up, failed int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			_, _ = w.Write([]byte(`{"error":""}`))
		case "/version":
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
		case "/metrics":
			_, _ = fmt.Fprintf(w, "input_connection_up{label=\"\",path=\"root.input\"} %d\n", up)
			_, _ = fmt.Fprintf(w, "input_connection_failed{label=\"\",path=\"root.input\"} %d\n", failed)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: "benthos-1", DesiredFSMState: "active"},
		MetricsPort:       uint16(srv.Listener.Addr().(*net.TCPAddr).Port),
	}
	deps := &benthosMonitorDeps{client: &http.Client{Timeout: 400 * time.Millisecond}}

	if _, err := Poll(context.Background(), deps, cfg); err != nil {
		t.Fatalf("first Poll errored: %v", err)
	}
	failed = 2
	status, err := Poll(context.Background(), deps, cfg)
	if err != nil {
		t.Fatalf("second Poll errored: %v", err)
	}
	if want := "benthos reports 1 input not connected"; status.Failures == nil {
		t.Fatalf("second Poll: status.Failures is nil, want %q", want)
	} else if ok, reason := status.Failures.ErrorFree(); ok || reason != want {
		t.Fatalf("input failed twice on the wire: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveReconnectReleasesOnJitteredScrapes: real scrapes are not exactly
// one second apart. With stamps alternating 993ms and 1007ms apart, the signal
// still releases within three scrapes of the reconnect.
func TestObserveReconnectReleasesOnJitteredScrapes(t *testing.T) {
	d := &benthosMonitorDeps{}
	at := failureT
	step := func(i int, m benthosmonitorservice.Metrics) (bool, string) {
		if i%2 == 0 {
			at = at.Add(993 * time.Millisecond)
		} else {
			at = at.Add(1007 * time.Millisecond)
		}
		_, v := observe(d, testPort, m, at)

		return v.ErrorFree()
	}

	step(0, connScrape(in(conn{}), nil))
	for i := 1; i <= 20; i++ {
		if ok, _ := step(i, connScrape(in(conn{failed: int64(i)}), nil)); ok {
			t.Fatalf("setup: scrape %d while down reads error-free", i)
		}
	}
	var ok bool
	var reason string
	for i := 21; i <= 23; i++ {
		ok, reason = step(i, connScrape(in(conn{up: 1, failed: 20}), nil))
	}
	if !ok {
		t.Fatalf("three jittered scrapes after the reconnect: ErrorFree() = (false, %q), want true", reason)
	}
}

// TestObserveReconnectReleasesAfterAScrapePause: scrapes pause for 30
// seconds, longer than the connection window, while the input is down. Once
// scraping resumes every second and benthos has reconnected, the signal
// releases within one window.
func TestObserveReconnectReleasesAfterAScrapePause(t *testing.T) {
	d := &benthosMonitorDeps{}
	at := failureT
	obs := func(gap time.Duration, c conn) (bool, string) {
		at = at.Add(gap)
		_, v := observe(d, testPort, connScrape(in(c), nil), at)

		return v.ErrorFree()
	}

	obs(0, conn{})
	for i := 1; i <= 5; i++ {
		obs(time.Second, conn{failed: int64(i)})
	}
	if ok, _ := obs(30*time.Second, conn{failed: 6}); ok {
		t.Fatalf("setup: down after the pause reads error-free")
	}
	var ok bool
	var reason string
	for i := 0; i < 11 && !ok; i++ {
		ok, reason = obs(time.Second, conn{up: 1, failed: 6})
	}
	if !ok {
		t.Fatalf("11 scrapes after the reconnect: ErrorFree() = (false, %q), want true", reason)
	}
}

// TestObserveSecondOutageReportsItsOwnStart: on jittered scrapes, an input
// that reconnects and drops again later is reported from the second drop.
// The first outage's signal must have released in between.
func TestObserveSecondOutageReportsItsOwnStart(t *testing.T) {
	d := &benthosMonitorDeps{}
	at := failureT
	i := 0
	obs := func(c conn) benthosmonitorservice.FailureVerdict {
		if i%2 == 0 {
			at = at.Add(993 * time.Millisecond)
		} else {
			at = at.Add(1007 * time.Millisecond)
		}
		i++
		_, v := observe(d, testPort, connScrape(in(c), nil), at)

		return v
	}

	obs(conn{up: 1})
	for k := 0; k < 5; k++ {
		obs(conn{up: 1, lost: 1})
	}
	for k := 0; k < 20; k++ {
		obs(conn{up: 2, lost: 1})
	}
	v := obs(conn{up: 2, lost: 2})
	secondDrop := at
	if len(v.Fired) != 1 {
		t.Fatalf("second drop: fired entries = %d, want 1", len(v.Fired))
	}
	if !v.Fired[0].Since.Equal(secondDrop) {
		t.Fatalf("second drop: entry Since = %v, want the second drop at %v", v.Fired[0].Since, secondDrop)
	}
}

// TestObserveDropSoonAfterReconnectIsReported: an input that drops again a
// few seconds after it reconnected is reported at once. The signal released
// on the reconnect, and its cooldown must not hide the new drop.
func TestObserveDropSoonAfterReconnectIsReported(t *testing.T) {
	d := &benthosMonitorDeps{}
	at := failureT
	obs := func(c conn) (bool, string) {
		at = at.Add(time.Second)
		_, v := observe(d, testPort, connScrape(in(c), nil), at)

		return v.ErrorFree()
	}

	obs(conn{up: 1})
	for k := 0; k < 15; k++ {
		obs(conn{up: 1, lost: 1})
	}
	for k := 0; k < 3; k++ {
		if ok, reason := obs(conn{up: 2, lost: 1}); k == 2 && !ok {
			t.Fatalf("setup: three scrapes after the reconnect: ErrorFree() = (false, %q), want true", reason)
		}
	}
	want := "benthos reports 1 input not connected"
	if ok, reason := obs(conn{up: 2, lost: 2}); ok || reason != want {
		t.Fatalf("drop 4s after the reconnect: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveRestartWhileDownReportsFromTheRestart: an input is down, benthos
// restarts, and the input is still down. The entry's Since is the first scrape
// after the restart, because the scrapes before it came from another process.
func TestObserveRestartWhileDownReportsFromTheRestart(t *testing.T) {
	d := &benthosMonitorDeps{}
	at := failureT
	obs := func(m benthosmonitorservice.Metrics) benthosmonitorservice.FailureVerdict {
		at = at.Add(time.Second)
		_, v := observe(d, testPort, m, at)

		return v
	}
	// restarted is a scrape of the new process: its received counter starts
	// again below the 10 that connScrape sets.
	restarted := func(c conn) benthosmonitorservice.Metrics {
		m := connScrape(in(c), nil)
		inst := m.Inputs["root.input"]
		inst.Received = 1
		m.Inputs["root.input"] = inst

		return m
	}

	obs(connScrape(in(conn{}), nil))
	for k := 1; k <= 3; k++ {
		obs(connScrape(in(conn{failed: int64(k)}), nil))
	}
	obs(restarted(conn{failed: 1}))
	restartAt := at
	v := obs(restarted(conn{failed: 2}))
	if len(v.Fired) != 1 {
		t.Fatalf("still down after the restart: fired entries = %d, want 1", len(v.Fired))
	}
	if !v.Fired[0].Since.Equal(restartAt) {
		t.Fatalf("still down after the restart: entry Since = %v, want the first scrape after the restart at %v", v.Fired[0].Since, restartAt)
	}
}
