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

	It("returns an error and a non-open status when the port is unreachable", func() {
		// Port 0 is not connectable, so the dial fails deterministically with no
		// listener to bind/free (avoids a port-reuse race between close and dial).
		cfg := newNmapConfig("127.0.0.1", 0)

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		status, err := fsmv2nmap.Poll(ctx, struct{}{}, cfg)
		Expect(err).To(HaveOccurred())
		Expect(status.PortState).NotTo(Equal("open"))
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
