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

package fsmv2datacontract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
)

func TestDataContract(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Contract Monitor Suite")
}

var _ = Describe("Poll", func() {
	var (
		ctx      context.Context
		d        *Deps
		server   *httptest.Server
		subjects []string
	)

	pump := func(shape string) config.FullConfig {
		return config.FullConfig{DataContractsV2: []config.DataContractV2Config{{
			Name: "pump",
			Versions: map[string]config.DataModelVersion{
				"v1": {Structure: map[string]config.Field{"count": {PayloadShape: shape}}},
			},
		}}}
	}

	BeforeEach(func() {
		ctx = context.Background()
		subjects = []string{"_pump_v1-timeseries-number"}
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(subjects)
		}))
		d = &Deps{
			configManager: config.NewMockConfigManager().WithConfig(pump("timeseries-number")),
			translator:    redpanda.NewSchemaRegistry(),
			subjectsURL:   server.URL,
			missingSince:  map[string]time.Time{},
			lastOK:        time.Now(),
		}
	})

	AfterEach(func() { server.Close() })

	It("reports nothing when every expected subject is registered", func() {
		Expect(Poll(ctx, d, struct{}{})).To(Equal(Status{}))
	})

	It("reports a missing subject only after the grace period, and clears it at once", func() {
		subjects = nil
		Expect(Poll(ctx, d, struct{}{})).To(Equal(Status{}))

		d.missingSince["_pump_v1-timeseries-number"] = time.Now().Add(-missingGrace)
		Expect(Poll(ctx, d, struct{}{})).To(Equal(Status{Missing: []string{"_pump_v1-timeseries-number"}}))

		subjects = []string{"_pump_v1-timeseries-number"}
		Expect(Poll(ctx, d, struct{}{})).To(Equal(Status{}))
		Expect(d.missingSince).To(BeEmpty())
	})

	It("reports a contract that cannot be translated", func() {
		d.configManager = config.NewMockConfigManager().WithConfig(pump("no-such-shape"))
		Expect(Poll(ctx, d, struct{}{})).To(Equal(Status{Untranslated: []string{"_pump_v1-"}}))
	})

	It("keeps the last status while the registry is briefly unreachable, then fails", func() {
		server.Close()

		Expect(Poll(ctx, d, struct{}{})).To(Equal(Status{}))

		d.lastOK = time.Now().Add(-missingGrace)
		_, err := Poll(ctx, d, struct{}{})
		Expect(err).To(MatchError(ContainSubstring("schema registry")))
	})
})
