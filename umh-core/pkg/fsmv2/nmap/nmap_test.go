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
	"encoding/binary"
	"net"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	fsmv2nmap "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
)

const (
	slowDNSHost  = "slow-dns.test"
	slowDNSDelay = 2500 * time.Millisecond
	// observationTimeout is the supervisor's DefaultObservationTimeout, the
	// budget a connection check had when it ran inside Poll.
	observationTimeout = 2200 * time.Millisecond
)

// newNmapConfig builds a valid config.NmapConfig pointing the check at target:port.
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

// checkOnce runs one probe with probeCtx and then one Poll, the way the
// framework runs them for a worker.
func checkOnce(probeCtx context.Context, cfg config.NmapConfig) (fsmv2nmap.NmapStatus, error) {
	nmapDeps := fsmv2nmap.NewDeps(deps.Identity{}, nil)
	fsmv2nmap.Probe(probeCtx, nmapDeps, cfg)

	return fsmv2nmap.Poll(context.Background(), nmapDeps, cfg)
}

// serveSlowDNS answers every A query for slowDNSHost with 127.0.0.1 after
// slowDNSDelay, and every other query with an empty answer after the same delay.
func serveSlowDNS(conn net.PacketConn) {
	buffer := make([]byte, 1500)

	for {
		n, from, err := conn.ReadFrom(buffer)
		if err != nil {
			return
		}

		query := append([]byte(nil), buffer[:n]...)

		go answerSlowly(conn, from, query)
	}
}

const (
	dnsHeaderLength = 12
	dnsTypeA        = 1
)

// answerSlowly replies to one DNS query after slowDNSDelay. An A query gets
// 127.0.0.1; any other type gets an empty answer. The reply drops the query's
// additional records, such as the EDNS0 record Go's resolver sends.
// https://www.rfc-editor.org/rfc/rfc1035#section-4.1
func answerSlowly(conn net.PacketConn, from net.Addr, query []byte) {
	questionEnd, questionType, parsed := parseQuestion(query)
	if !parsed {
		return
	}

	time.Sleep(slowDNSDelay)

	response := append([]byte(nil), query[:questionEnd]...)
	response[2] = 0x84 // QR (response) and AA (authoritative), opcode QUERY
	response[3] = 0x00 // RCODE no error
	binary.BigEndian.PutUint16(response[8:10], 0)
	binary.BigEndian.PutUint16(response[10:12], 0)

	if questionType == dnsTypeA {
		binary.BigEndian.PutUint16(response[6:8], 1)
		response = append(response,
			0xc0, dnsHeaderLength, // name: pointer to the question name
			0x00, dnsTypeA, 0x00, 0x01, // type A, class IN
			0x00, 0x00, 0x00, 0x3c, // TTL 60 s
			0x00, 0x04, 127, 0, 0, 1, // RDATA 127.0.0.1
		)
	} else {
		binary.BigEndian.PutUint16(response[6:8], 0)
	}

	_, _ = conn.WriteTo(response, from)
}

// parseQuestion returns where the single question of a DNS query ends and its type.
func parseQuestion(query []byte) (questionEnd int, questionType uint16, parsed bool) {
	offset := dnsHeaderLength
	for offset < len(query) && query[offset] != 0 {
		offset += int(query[offset]) + 1
	}

	questionEnd = offset + 5
	if questionEnd > len(query) {
		return 0, 0, false
	}

	return questionEnd, binary.BigEndian.Uint16(query[offset+1 : offset+3]), true
}

func slowResolver(dnsServer net.PacketConn) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer

			return dialer.DialContext(ctx, "udp", dnsServer.LocalAddr().String())
		},
	}
}

var _ = Describe("Nmap connection check", func() {
	It("reports the port open when a TCP listener is accepting", func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = listener.Close() }()

		host, port := hostPort(listener.Addr().String())

		status, err := checkOnce(context.Background(), newNmapConfig(host, port))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal("open"))
		Expect(status.Port).To(Equal(port))
		Expect(status.Target).To(Equal(host))
		Expect(status.IsRunning).To(BeTrue())
		Expect(status.LatencyMs).To(BeNumerically(">=", 0))
	})

	It("reports a closed port without an error when the connection is refused", func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		host, port := hostPort(listener.Addr().String())
		Expect(listener.Close()).To(Succeed())

		status, err := checkOnce(context.Background(), newNmapConfig(host, port))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal("closed"))
		Expect(status.IsRunning).To(BeFalse())
	})

	It("reports a closed port without an error when the probe's deadline is already exceeded", func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = listener.Close() }()

		host, port := hostPort(listener.Addr().String())

		expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
		defer cancel()

		status, err := checkOnce(expiredCtx, newNmapConfig(host, port))
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal("closed"))
	})

	It("records the time the probe started", func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = listener.Close() }()

		host, port := hostPort(listener.Addr().String())

		before := time.Now()
		status, err := checkOnce(context.Background(), newNmapConfig(host, port))
		after := time.Now()

		Expect(err).NotTo(HaveOccurred())
		Expect(status.ScannedAt).To(BeTemporally(">=", before))
		Expect(status.ScannedAt).To(BeTemporally("<=", after))
	})

	It("reports an error and no port state before any probe has finished", func() {
		cfg := newNmapConfig("127.0.0.1", 502)

		status, err := fsmv2nmap.Poll(context.Background(), fsmv2nmap.NewDeps(deps.Identity{}, nil), cfg)
		Expect(err).To(HaveOccurred())
		Expect(status.PortState).To(BeEmpty())
		Expect(status.Target).To(Equal(cfg.NmapServiceConfig.Target))
		Expect(status.ScannedAt).To(BeZero())
	})

	It("records nothing when the probe is cancelled, so a stopping worker reports no port state", func() {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		status, err := checkOnce(cancelledCtx, newNmapConfig("127.0.0.1", 502))
		Expect(err).To(HaveOccurred())
		Expect(status.PortState).To(BeEmpty())
	})

	It("does not report a result for the previous target after the target changed", func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = listener.Close() }()

		host, port := hostPort(listener.Addr().String())
		nmapDeps := fsmv2nmap.NewDeps(deps.Identity{}, nil)
		fsmv2nmap.Probe(context.Background(), nmapDeps, newNmapConfig(host, port))

		status, err := fsmv2nmap.Poll(context.Background(), nmapDeps, newNmapConfig("192.0.2.10", port))
		Expect(err).To(HaveOccurred())
		Expect(status.PortState).To(BeEmpty())
		Expect(status.Target).To(Equal("192.0.2.10"))
	})

	It("reports an error but keeps the last result when it is older than the age limit", func() {
		cfg := newNmapConfig("127.0.0.1", 502)
		nmapDeps := fsmv2nmap.NewDeps(deps.Identity{}, nil)
		lastResult := fsmv2nmap.NmapStatus{Target: "127.0.0.1", Port: 502, PortState: "open", IsRunning: true}
		fsmv2nmap.RecordCheck(nmapDeps, lastResult, time.Now().Add(-fsmv2nmap.ResultMaxAge-time.Second))

		status, err := fsmv2nmap.Poll(context.Background(), nmapDeps, cfg)
		Expect(err).To(HaveOccurred())
		Expect(status).To(Equal(lastResult))
	})

	It("reports a result that is younger than the age limit", func() {
		cfg := newNmapConfig("127.0.0.1", 502)
		nmapDeps := fsmv2nmap.NewDeps(deps.Identity{}, nil)
		lastResult := fsmv2nmap.NmapStatus{Target: "127.0.0.1", Port: 502, PortState: "open", IsRunning: true}
		fsmv2nmap.RecordCheck(nmapDeps, lastResult, time.Now().Add(-fsmv2nmap.ResultMaxAge+time.Second))

		status, err := fsmv2nmap.Poll(context.Background(), nmapDeps, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(lastResult))
	})

	It("reports the port open when the DNS lookup takes longer than an observation", func() {
		dnsServer, err := net.ListenPacket("udp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = dnsServer.Close() }()

		go serveSlowDNS(dnsServer)

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = listener.Close() }()

		_, port := hostPort(listener.Addr().String())
		cfg := newNmapConfig(slowDNSHost, port)
		nmapDeps := fsmv2nmap.NewDepsWithResolver(slowResolver(dnsServer))

		probeStart := time.Now()

		fsmv2nmap.Probe(context.Background(), nmapDeps, cfg)

		probeDuration := time.Since(probeStart)

		status, err := fsmv2nmap.Poll(context.Background(), nmapDeps, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.PortState).To(Equal("open"))
		Expect(probeDuration).To(BeNumerically(">", observationTimeout))
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
