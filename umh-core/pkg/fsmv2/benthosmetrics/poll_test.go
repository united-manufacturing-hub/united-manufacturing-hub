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

package fsmv2benthosmetrics

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics/benthosmetricstest"
)

var _ = Describe("Poll", func() {
	// Poll returns an error without sending a request when its deps carry no
	// Tracker, which only happens when the worker was registered without
	// NewDeps.
	It("refuses to scrape with unbound deps", func() {
		s := benthosmetricstest.NewServer()
		defer s.Close()

		cfg := Config{MetricsPort: s.Port()}

		By("nil deps")
		sample, err := Poll(context.Background(), nil, cfg)
		Expect(err).To(HaveOccurred(), "Poll must not scrape without deps")
		Expect(sample).To(Equal(Scrape{}), "a refused scrape must return the zero Scrape")

		By("deps with no Tracker")
		sample, err = Poll(context.Background(), &benthosDeps{}, cfg)
		Expect(err).To(HaveOccurred(), "Poll must not scrape without a Tracker")
		Expect(sample).To(Equal(Scrape{}), "a refused scrape must return the zero Scrape")

		// No request may have left for the healthy Benthos above; only the
		// deps check can have failed those two calls.
		Expect(s.Requests()).To(BeEmpty(), "the refused scrapes must not send any request")

		// The control: the guard must not fire on deps with a Tracker.
		sample, err = Poll(context.Background(), &benthosDeps{tracker: benthosmetrics.NewTracker(time.Now)}, cfg)
		Expect(err).NotTo(HaveOccurred(), "the deps guard must not fire on bound deps")
		Expect(sample.Probes.Version).To(Equal("1.2.3"),
			"bound deps must scrape the Benthos the calls above refused")
	})

	// A failed scrape returns the last good sample with the error, so the
	// worker's stored sample keeps vouching for liveness until it ages out.
	It("returns the last good sample when a scrape fails", func() {
		s := benthosmetricstest.NewServer()
		cfg := Config{MetricsPort: s.Port()}
		d := &benthosDeps{tracker: benthosmetrics.NewTracker(time.Now)}

		good, err := Poll(context.Background(), d, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(good.Probes.PingAlive).To(BeTrue())

		s.Close()

		failed, err := Poll(context.Background(), d, cfg)
		Expect(err).To(HaveOccurred(), "the closed Benthos must fail the scrape")
		Expect(failed).To(Equal(good), "a failed scrape must return the last good sample")
	})

	// Poll threads ctx into every request, so a cancelled context fails the
	// scrape instead of silently scraping in the background.
	It("fails a cancelled context", func() {
		s := benthosmetricstest.NewServer()
		defer s.Close()

		cfg := Config{MetricsPort: s.Port()}
		deps := &benthosDeps{tracker: benthosmetrics.NewTracker(time.Now)}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := Poll(ctx, deps, cfg)
		Expect(err).To(HaveOccurred(), "a cancelled context must fail the scrape")
	})
})
