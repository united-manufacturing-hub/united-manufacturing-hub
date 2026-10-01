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

package fsmv2timescale

import (
	"context"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
)

type classification struct {
	serverAnswered bool
	authRejected   bool
}

func classify(err error) classification {
	return classification{serverAnswered: serverAnswered(err), authRejected: authRejected(err)}
}

// connectionCheckError runs the SELECT 1 that Poll runs, through a pool built the
// way Poll builds one.
func connectionCheckError(cfg config.HistorianConfig) error {
	pool, err := (&poolHolder{}).get(cfg.WithDefaults().Timescale.ToDSN())
	if err != nil {
		Fail("build the pool: " + err.Error())

		return nil
	}

	DeferCleanup(pool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var one int

	return pool.QueryRow(ctx, "SELECT 1").Scan(&one)
}

var _ = Describe("The errors the historian scenario injects", func() {
	It("classify a refused connection like the real one", func() {
		realErr := connectionCheckError(config.HistorianConfig{Timescale: config.TimescaleConfig{
			Host:    "127.0.0.1",
			Port:    closedPort(),
			SSLMode: config.HistorianSSLModeDisable,
		}})

		Expect(realErr).To(MatchError(syscall.ECONNREFUSED), "nothing listens on the port")
		Expect(timescalemetrics.ErrFakeConnectionRefused).To(MatchError(syscall.ECONNREFUSED))
		Expect(classify(timescalemetrics.ErrFakeConnectionRefused)).To(Equal(classify(realErr)), realErr.Error())
	})
})
