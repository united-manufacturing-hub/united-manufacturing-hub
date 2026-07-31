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
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("Error classification", func() {
	type tc struct {
		err          error
		wantAnswered bool
		wantAuth     bool
	}

	DescribeTable("serverAnswered and authRejected",
		func(t tc) {
			Expect(serverAnswered(t.err)).To(Equal(t.wantAnswered), "serverAnswered")
			Expect(authRejected(t.err)).To(Equal(t.wantAuth), "authRejected")
		},
		Entry("auth: invalid password (28P01)",
			tc{err: &pgconn.PgError{Code: "28P01", Message: "password authentication failed"}, wantAnswered: true, wantAuth: true}),
		Entry("auth: invalid authorization class (28000)",
			tc{err: &pgconn.PgError{Code: "28000", Message: "invalid authorization specification"}, wantAnswered: true, wantAuth: true}),
		Entry("unknown database: postgres invalid catalog (3D000)",
			tc{err: &pgconn.PgError{Code: "3D000", Message: `database "nope" does not exist`}, wantAnswered: true, wantAuth: true}),
		Entry("unknown database: pgbouncer missing database (08P01)",
			tc{err: &pgconn.PgError{Code: "08P01", Message: "no such database: nope"}, wantAnswered: true, wantAuth: true}),
		Entry("protocol violation: unrelated 08P01 reachable but not auth",
			tc{err: &pgconn.PgError{Code: "08P01", Message: "invalid startup packet"}, wantAnswered: true, wantAuth: false}),
		Entry("server error: syntax error (42601) reachable but not auth",
			tc{err: &pgconn.PgError{Code: "42601", Message: "syntax error"}, wantAnswered: true, wantAuth: false}),
		Entry("timeout: context deadline exceeded unreachable",
			tc{err: context.DeadlineExceeded, wantAnswered: false, wantAuth: false}),
		Entry("network: plain error unreachable",
			tc{err: errors.New("dial tcp: connection refused"), wantAnswered: false, wantAuth: false}),
		Entry("wrapped: pgbouncer missing database through fmt.Errorf",
			tc{err: fmt.Errorf("timescale query: %w", &pgconn.PgError{Code: "08P01", Message: "no such database: nope"}), wantAnswered: true, wantAuth: true}),
	)
})

var _ = Describe("newDeps", func() {
	It("hands every instance the same pool holder", func() {
		a, b := newDeps(deps.Identity{ID: "timescale-001", HierarchyPath: "parent-a/timescale-001"}),
			newDeps(deps.Identity{ID: "timescale-001", HierarchyPath: "parent-b/timescale-001"})

		Expect(a.pool).NotTo(BeNil(), "the holder exists, so the comparison below is not vacuous")
		Expect(a.pool).To(BeIdenticalTo(b.pool),
			"one holder for the process: a per-instance pool would leak its pgxpool goroutine on every despawn, and nothing in the framework closes it")
		Expect(a.Logger).NotTo(BeNil(), "Poll dereferences the logger on every tick")
	})
})

var _ = Describe("Poll", func() {
	It("reports unreachable with authentication unknown when the DSN does not parse", func() {
		// A colon in the host reaches Poll in production: TimescaleConfig.Validate
		// only requires host to be non-empty, and net.JoinHostPort then brackets the
		// value into what pgx reads as a malformed IPv6 literal. Pool creation parses
		// the DSN without dialling, so this covers the early return with no database
		// and no network.
		cfg := config.HistorianConfig{Timescale: config.TimescaleConfig{
			Host:     "host:with:colons",
			Password: "unlikely-to-appear-by-accident",
		}}

		status, err := Poll(context.Background(),
			newDeps(deps.Identity{ID: "timescale", HierarchyPath: "historian/timescale"}), cfg)

		Expect(err).To(MatchError(ContainSubstring("parse timescale dsn")),
			"the error wraps the parse failure, so the degraded verdict names the cause")
		Expect(err).NotTo(MatchError(ContainSubstring(cfg.Timescale.Password)),
			"the error quotes the DSN, which carries the password: pgx masks it, and this error is logged")
		Expect(status.Reachable).To(BeFalse(), "nothing was dialled, so the endpoint is not proven reachable")
		Expect(status.Auth).To(Equal(models.TimescaleAuthUnknown),
			"no server answered, so the credentials stay unverified rather than rejected")
	})
})
