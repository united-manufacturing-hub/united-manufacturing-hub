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
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
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

// idUnder builds the identity the supervisor would hand a timescale child of the
// named parent. A child's ID is "<name>-001", so two children of the same name
// under different parents share ID and Name, and only the hierarchy path tells
// them apart.
func idUnder(parent string) deps.Identity {
	return deps.Identity{
		ID:            InstanceName + "-001",
		Name:          InstanceName,
		WorkerType:    WorkerType,
		HierarchyPath: parent + "(historian)/" + InstanceName + "-001(" + WorkerType + ")",
	}
}

var _ = Describe("newDeps, the helper", func() {
	It("hands every instance the same pool holder", func() {
		a, b := newDeps(idUnder("parent-a")), newDeps(idUnder("parent-b"))

		Expect(a.pool).NotTo(BeNil(), "the holder exists, so the comparison below is not vacuous")
		Expect(a.pool).To(BeIdenticalTo(b.pool),
			"one holder for the process: a per-instance pool would leak its pgxpool goroutine on every despawn after a poll, and nothing in the framework closes it")
		Expect(a.Logger).NotTo(BeNil(), "Poll dereferences the logger on every tick")
	})
})

var _ = Describe("the registered worker type", func() {
	// poolOf builds one instance the way production does — through the factory
	// init() registered with, not by calling newDeps — and reads the pool holder
	// out of the deps value the worker kept. simpleWorker.instDeps is unexported
	// and lives in another package, so reflect is the only reader; IsNil and
	// Pointer need no CanInterface.
	poolOf := func(id deps.Identity) reflect.Value {
		w, err := factory.NewWorkerByType(WorkerType, id, deps.NewNopFSMLogger(), nil, nil)
		Expect(err).NotTo(HaveOccurred(), "init() left an instantiable factory for the worker type")

		instDeps := reflect.ValueOf(w).Elem().FieldByName("instDeps")
		Expect(instDeps.IsValid()).To(BeTrue(), "the worker keeps its poll deps in instDeps")

		pool := instDeps.FieldByName("pool")
		Expect(pool.IsValid()).To(BeTrue(), "the poll deps carry the pool holder")

		return pool
	}

	It("hands every instance built through the factory the same pool holder", func() {
		// Asserting on newDeps alone pins the helper, not the registration: a
		// spec that calls newDeps and then overwrites pool with a fresh holder
		// passes that assertion while restoring the goroutine leak. This one
		// goes through the path a spawning supervisor takes.
		a, b := poolOf(idUnder("parent-a")), poolOf(idUnder("parent-b"))

		Expect(a.IsNil()).To(BeFalse(), "the holder exists, so the comparison below is not vacuous")
		Expect(a.Pointer()).To(Equal(b.Pointer()),
			"one holder for the process: a per-instance pool would leak its pgxpool goroutine on every despawn after a poll, and nothing in the framework closes it")
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
