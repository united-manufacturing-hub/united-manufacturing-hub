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
	"net"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2nmap "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
	nmapservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

const unresolvableHost = "connection-check.invalid"

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

var _ = Describe("Nmap Poll", func() {
	It("reports the port open when a TCP listener is accepting", func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = ln.Close() }()

		host, port := hostPort(ln.Addr().String())
		cfg := newNmapConfig(host, port)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, cfg)
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

		st, err := fsmv2nmap.Poll(ctx, struct{}{}, cfg)
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

		openStatus, err := fsmv2nmap.Poll(openCtx, struct{}{}, newNmapConfig(openHost, openPort))
		Expect(err).NotTo(HaveOccurred())

		refusedCtx, cancelRefused := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancelRefused()

		refusedStatus, err := fsmv2nmap.Poll(refusedCtx, struct{}{}, newNmapConfig(refusedHost, refusedPort))
		Expect(err).NotTo(HaveOccurred())

		cancelledCtx, cancelNow := context.WithCancel(context.Background())
		cancelNow()

		cancelledStatus, err := fsmv2nmap.Poll(cancelledCtx, struct{}{}, newNmapConfig(openHost, openPort))
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

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, cfg)
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

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal("closed"))
		Expect(status.IsRunning).To(BeFalse())
	})

	It("records no failure when the port is open", func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = listener.Close() }()

		host, port := hostPort(listener.Addr().String())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, newNmapConfig(host, port))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Dial.Failure).To(BeEmpty())
	})

	It("records a refused connection as refused", func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		host, port := hostPort(listener.Addr().String())
		Expect(listener.Close()).To(Succeed())

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, newNmapConfig(host, port))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Dial.Failure).To(Equal(nmapservice.DialFailureRefused))
	})

	It("records a deadline hit while connecting to an IP address as a connect timeout", func() {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, newNmapConfig("127.0.0.1", 1))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Dial.Failure).To(Equal(nmapservice.DialFailureConnectTimeout))
	})

	It("records a deadline hit while looking up a hostname as a DNS timeout", func() {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, newNmapConfig(unresolvableHost, 80))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Dial.Failure).To(Equal(nmapservice.DialFailureDNSTimeout))
	})

	It("records a hostname that does not resolve as a DNS error", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, newNmapConfig(unresolvableHost, 80))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Dial.Failure).To(Equal(nmapservice.DialFailureDNSError))
	})

	It("splits the time of an open dial into the lookup and the connect", func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = listener.Close() }()

		_, port := hostPort(listener.Addr().String())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		before := time.Now()
		status, err := fsmv2nmap.Poll(ctx, struct{}{}, newNmapConfig("localhost", port))
		wallClockMs := float64(time.Since(before).Microseconds()) / 1000.0

		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal("open"))
		Expect(status.Dial.ResolveMs).To(BeNumerically(">", 0))
		Expect(status.Dial.ConnectMs).To(BeNumerically(">", 0))
		Expect(status.Dial.ResolveMs + status.Dial.ConnectMs).To(BeNumerically("<=", wallClockMs))
	})

	It("attributes the whole time of a failed lookup to the lookup", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, newNmapConfig(unresolvableHost, 80))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Dial.ResolveMs).To(BeNumerically(">", 0))
		Expect(status.Dial.ConnectMs).To(BeZero())
	})

	It("returns quickly with an error when the context is already cancelled", func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = ln.Close() }()

		host, port := hostPort(ln.Addr().String())
		cfg := newNmapConfig(host, port)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, cfg)
		Expect(err).To(HaveOccurred())
		Expect(status.PortState).NotTo(Equal("open"))
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
