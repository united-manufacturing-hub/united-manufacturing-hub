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

package fsmv2nmap_test

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	fsmv2nmap "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
	nmapservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

// newNmapConfig builds a valid config.NmapConfig pointing Poll at target:port.
func newNmapConfig(target string, port uint16) config.NmapConfig {
	return config.NmapConfig{
		NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
			Target: target,
			Port:   port,
		},
	}
}

// hostPort splits a "host:port" listener address into its host and uint16 port.
func hostPort(addr string) (string, uint16) {
	host, portStr, err := net.SplitHostPort(addr)
	Expect(err).NotTo(HaveOccurred())

	p, err := strconv.ParseUint(portStr, 10, 16)
	Expect(err).NotTo(HaveOccurred())

	return host, uint16(p)
}

// newPollDeps builds the deps value Poll dials through, via newDeps. The
// framework's BaseDependencies are not part of nmap's deps, so a nil one is
// all there is to hand.
func newPollDeps(m map[string]any) fsmv2nmap.Deps {
	id := deps.Identity{ID: "nmap-poll", WorkerType: fsmv2nmap.WorkerType}

	return fsmv2nmap.NewDepsForTest(id, nil, m)
}

// nmapID builds the identity a supervisor hands an nmap worker, with the
// hierarchy path the supervisor reports it under.
func nmapID() deps.Identity {
	return deps.Identity{
		ID:            "nmap-001",
		Name:          "nmap",
		WorkerType:    fsmv2nmap.WorkerType,
		HierarchyPath: "nmap-001(nmap)",
	}
}

// fakeDialer records each address it dials. Its net.Pipe answer makes Poll
// report the port open with nothing listening.
type fakeDialer struct {
	addresses []string
	err       error
}

func (f *fakeDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	f.addresses = append(f.addresses, address)

	if f.err != nil {
		return nil, f.err
	}

	oneEnd, otherEnd := net.Pipe()
	_ = otherEnd.Close()

	return oneEnd, nil
}

var _ = Describe("Nmap Poll", func() {
	It("reports the port open when a TCP listener is accepting", func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = ln.Close() }()

		host, port := hostPort(ln.Addr().String())
		cfg := newNmapConfig(host, port)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, newPollDeps(nil), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal("open"))
		Expect(status.Port).To(Equal(port))
		Expect(status.IsRunning).To(BeTrue())
		Expect(status.LatencyMs).To(BeNumerically(">=", 0))
	})

	It("records the target and port it dialed", func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = ln.Close() }()

		host, p := hostPort(ln.Addr().String())
		cfg := newNmapConfig(host, p)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		st, err := fsmv2nmap.Poll(ctx, newPollDeps(nil), cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(st.Target).To(Equal(host), "Poll must record the target it dialed")
		Expect(st.Port).To(Equal(p), "Poll must record the port it dialed")
	})

	It("records the scan start time and the target on the open, refused and cancelled paths", func() {
		openListener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = openListener.Close() }()

		openHost, openPort := hostPort(openListener.Addr().String())

		refusedListener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		refusedHost, refusedPort := hostPort(refusedListener.Addr().String())
		Expect(refusedListener.Close()).To(Succeed())

		before := time.Now()

		openCtx, cancelOpen := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelOpen()

		openStatus, err := fsmv2nmap.Poll(openCtx, newPollDeps(nil), newNmapConfig(openHost, openPort))
		Expect(err).NotTo(HaveOccurred())

		refusedCtx, cancelRefused := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancelRefused()

		refusedStatus, err := fsmv2nmap.Poll(refusedCtx, newPollDeps(nil), newNmapConfig(refusedHost, refusedPort))
		Expect(err).NotTo(HaveOccurred())

		cancelledCtx, cancelNow := context.WithCancel(context.Background())
		cancelNow()

		cancelledStatus, err := fsmv2nmap.Poll(cancelledCtx, newPollDeps(nil), newNmapConfig(openHost, openPort))
		Expect(err).To(HaveOccurred())

		after := time.Now()

		paths := map[string]fsmv2nmap.NmapStatus{
			"open":      openStatus,
			"refused":   refusedStatus,
			"cancelled": cancelledStatus,
		}

		for path, status := range paths {
			Expect(status.ScannedAt).To(BeTemporally(">=", before), "%s path must record when the dial started", path)
			Expect(status.ScannedAt).To(BeTemporally("<=", after), "%s path must record when the dial started", path)
			Expect(status.Target).NotTo(BeEmpty(), "%s path must record the target it dialed", path)
		}
	})

	It("reports a closed port without an error when the connection is refused", func() {
		// Bind a listener to grab a free loopback port, then close it so the
		// kernel answers the dial with a TCP RST (connection refused). A refused
		// connection is a legitimate scan outcome, not a poll failure, so Poll
		// returns a nil error.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		host, port := hostPort(ln.Addr().String())
		Expect(ln.Close()).To(Succeed())

		cfg := newNmapConfig(host, port)

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, newPollDeps(nil), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal("closed"))
		Expect(status.IsRunning).To(BeFalse())
	})

	It("reports a closed port without an error when the deadline is already exceeded", func() {
		// A deadline (the collector's ObservationTimeout) is not a shutdown: the
		// dial fails with context.DeadlineExceeded, which must fall through to a
		// closed port, not the cancelled-context error path. An expired deadline
		// makes DialContext fail deterministically without a network round-trip.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = ln.Close() }()

		host, port := hostPort(ln.Addr().String())
		cfg := newNmapConfig(host, port)

		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, newPollDeps(nil), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal("closed"))
		Expect(status.IsRunning).To(BeFalse())
	})

	It("returns quickly with an error when the context is already cancelled", func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = ln.Close() }()

		host, port := hostPort(ln.Addr().String())
		cfg := newNmapConfig(host, port)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		status, err := fsmv2nmap.Poll(ctx, newPollDeps(nil), cfg)
		Expect(err).To(HaveOccurred())
		Expect(status.PortState).NotTo(Equal("open"))
	})
})

var _ = Describe("Nmap Poll dependencies", func() {
	It("dials through the dialer stored under its dependency key", func() {
		// The target is a loopback port nothing listens on, so a Poll that
		// still dials for real reports closed and the fake records nothing.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		host, port := hostPort(ln.Addr().String())
		Expect(ln.Close()).To(Succeed())

		fake := &fakeDialer{}

		m := map[string]any{}

		var dialer fsmv2nmap.Dialer = fake
		fsmv2config.SetDependency(m, fsmv2nmap.DialerKey, dialer)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, newPollDeps(m), newNmapConfig(host, port))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal(string(nmapservice.PortStateOpen)),
			"Poll must dial through the dialer from the dependency map, which answers")
		Expect(fake.addresses).To(Equal([]string{net.JoinHostPort(host, strconv.Itoa(int(port)))}),
			"Poll must hand the map's dialer exactly the target address")
	})

	It("reports a closed port when the injected dialer fails", func() {
		fake := &fakeDialer{err: errors.New("connection refused")}

		m := map[string]any{}

		var dialer fsmv2nmap.Dialer = fake
		fsmv2config.SetDependency(m, fsmv2nmap.DialerKey, dialer)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, newPollDeps(m), newNmapConfig("10.0.0.1", 502))
		Expect(err).NotTo(HaveOccurred(),
			"a failed dial is a scan outcome, not a poll failure")
		Expect(status.PortState).To(Equal(string(nmapservice.PortStateClosed)))
		Expect(fake.addresses).To(Equal([]string{net.JoinHostPort("10.0.0.1", "502")}),
			"Poll must hand the map's dialer exactly the target address")
	})
})

var _ = Describe("the registered nmap worker type", func() {
	// boundDepsOf builds one worker through the factory init() registered, as
	// production does, and returns the Deps the worker holds.
	boundDepsOf := func(id deps.Identity, dependencies map[string]any) fsmv2nmap.Deps {
		w, err := factory.NewWorkerByType(fsmv2nmap.WorkerType, id, deps.NewNopFSMLogger(), nil, dependencies)
		Expect(err).NotTo(HaveOccurred(), "init() left an instantiable factory for the worker type")
		Expect(w).NotTo(BeNil(), "an instance exists, so the reads below are not vacuous")

		dp, ok := w.(fsmv2.DependencyProvider)
		Expect(ok).To(BeTrue(), "the worker reports the deps Poll receives")

		bound, ok := dp.GetDependenciesAny().(fsmv2nmap.Deps)
		Expect(ok).To(BeTrue(), "the framework binds newDeps' return, unwrapped")

		return bound
	}

	It("dials through the dialer stored in the dependency map the worker was built with", func() {
		// init()'s NewDeps wiring is what hands a production worker the dialer
		// from its dependency map; a spec that builds Deps through
		// NewDepsForTest directly passes with that wiring broken.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		host, port := hostPort(ln.Addr().String())
		Expect(ln.Close()).To(Succeed())

		fake := &fakeDialer{}

		m := map[string]any{}

		var dialer fsmv2nmap.Dialer = fake
		fsmv2config.SetDependency(m, fsmv2nmap.DialerKey, dialer)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		bound := boundDepsOf(nmapID(), m)

		status, err := fsmv2nmap.Poll(ctx, bound, newNmapConfig(host, port))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal(string(nmapservice.PortStateOpen)),
			"Poll must dial through the dialer NewDeps read from the map the worker was built with")
		Expect(fake.addresses).To(Equal([]string{net.JoinHostPort(host, strconv.Itoa(int(port)))}),
			"the map's dialer must receive exactly the target address")
	})

	It("does not satisfy the injection interfaces supervisor/api.go asserts on the deps", func() {
		// Framework telemetry for nmap comes from the collector (PR #2678),
		// not from its dependencies.
		bound := boundDepsOf(nmapID(), nil)

		_, setsActionHistory := any(bound).(interface{ SetActionHistory([]deps.ActionResult) })
		Expect(setsActionHistory).To(BeFalse(),
			"nmap's deps carry only the dialer, so the collector's injection pass skips them")
	})
})

var _ = Describe("Nmap registration", func() {
	It("registers the nmap worker type on import", func() {
		Expect(fsmv2.LookupInitialState("nmap")).NotTo(BeNil())
	})

	It("registers a positive observation interval for the nmap worker type", func() {
		interval, ok := fsmv2.ObservationIntervalFor("nmap")
		Expect(ok).To(BeTrue())
		Expect(interval).To(BeNumerically(">", 0))
	})
})
