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
	"net"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// A polling spec fills the package global sharedPool with a live pool, and only
// Close stops that pool's health-check goroutine. Swapping a fresh holder in for
// each spec keeps one spec's pool out of every later spec, so the suite means the
// same thing in any order. The two sharing specs still hold: they compare the
// holders handed to two instances against each other, not against this package's
// initial value.
var _ = BeforeEach(func() {
	original, fresh := sharedPool, &poolHolder{}
	sharedPool = fresh

	DeferCleanup(func() {
		sharedPool = original

		if pool := pooledBy(fresh); pool != nil {
			pool.Close()
		}
	})
})

// pooledBy reads the pool a holder ended up with. poolHolder.get assigns it under
// the holder's mutex, so the read takes the same lock.
func pooledBy(h *poolHolder) *pgxpool.Pool {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.pool
}

// closedPort returns a loopback port with nothing listening on it: it binds one,
// reads back the port the kernel assigned, and closes the listener. Dialling it is
// refused immediately, so a spec can reach a real pool without a database.
func closedPort() uint16 {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred(), "a loopback listener is available")

	addr, ok := ln.Addr().(*net.TCPAddr)
	Expect(ok).To(BeTrue(), "a tcp listener reports a *net.TCPAddr")
	Expect(ln.Close()).To(Succeed())

	return uint16(addr.Port)
}

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

// baseUnder builds the BaseDependencies the framework hands newDeps for a child
// of the named parent. Production reaches newDeps only through InitBase's return
// value; a spec that calls newDeps directly has to build the equivalent.
func baseUnder(parent string) *deps.BaseDependencies {
	id := idUnder(parent)

	return deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, id)
}

var _ = Describe("newDeps, the helper", func() {
	It("hands every instance the same pool holder", func() {
		a := newDeps(idUnder("parent-a"), baseUnder("parent-a"))
		b := newDeps(idUnder("parent-b"), baseUnder("parent-b"))

		Expect(a.pool).NotTo(BeNil(), "the holder exists, so the comparison below is not vacuous")
		Expect(a.pool).To(BeIdenticalTo(b.pool),
			"one holder for the process: a per-instance pool would leak its pgxpool goroutine on every despawn after a poll, and nothing in the framework closes it")
	})

	It("keeps the BaseDependencies it was handed rather than building its own logger", func() {
		bd := baseUnder("parent-a")

		d := newDeps(idUnder("parent-a"), bd)

		Expect(d.BaseDependencies).To(BeIdenticalTo(bd),
			"the deps carry the framework's own BaseDependencies, so Poll's logger is the one enriched with this worker's identity, not a package global")
		Expect(d.GetLogger()).NotTo(BeNil(), "Poll dereferences the logger on every tick")
	})
})

var _ = Describe("the registered worker type", func() {
	// boundDepsOf builds one instance the way production does, through the factory
	// init() registered with rather than by calling newDeps, and returns what the
	// worker put in its deps slot.
	//
	// The simple framework wraps every worker's poll deps in an unexported type of
	// its own, so this package cannot name what comes back and reflect is the only
	// reader. The wrapper's *deps.BaseDependencies field is exported and reads out
	// with Interface(); the author's value sits behind the wrapper's unexported
	// inst field, which can be inspected (IsNil, Pointer) but not converted back to
	// a Deps.
	boundDepsOf := func(id deps.Identity) reflect.Value {
		w, err := factory.NewWorkerByType(WorkerType, id, deps.NewNopFSMLogger(), nil, nil)
		Expect(err).NotTo(HaveOccurred(), "init() left an instantiable factory for the worker type")

		dp, ok := w.(fsmv2.DependencyProvider)
		Expect(ok).To(BeTrue(), "the worker reports its poll deps through fsmv2.DependencyProvider")

		bound := reflect.ValueOf(dp.GetDependenciesAny())
		Expect(bound.Kind()).To(Equal(reflect.Ptr), "the framework binds a pointer to its wrapper")
		Expect(bound.IsNil()).To(BeFalse(), "the wrapper exists, so the reads below are not vacuous")

		return bound.Elem()
	}

	// instOf reads the author's Deps value out of the framework's wrapper.
	instOf := func(bound reflect.Value) reflect.Value {
		inst := bound.FieldByName("inst")
		Expect(inst.IsValid()).To(BeTrue(), "the wrapper keeps the author's poll value in inst")

		return inst
	}

	It("gives each instance the BaseDependencies the framework built for it", func() {
		id := idUnder("parent-a")

		bound := boundDepsOf(id)

		frameworkBD, ok := bound.FieldByName("BaseDependencies").Interface().(*deps.BaseDependencies)
		Expect(ok).To(BeTrue(), "the framework's wrapper carries the instance's BaseDependencies")
		Expect(frameworkBD).NotTo(BeNil(),
			"the collector reads framework metrics and action history off this, so the reads below are not vacuous")
		Expect(frameworkBD.GetWorkerType()).To(Equal(id.WorkerType),
			"the BaseDependencies was built from this instance's identity, not a fresh or shared one")
		Expect(frameworkBD.GetWorkerID()).To(Equal(id.ID))
		Expect(frameworkBD.GetHierarchyPath()).To(Equal(id.HierarchyPath))

		instBD := instOf(bound).FieldByName("BaseDependencies")
		Expect(instBD.IsNil()).To(BeFalse(),
			"Poll logs through this on every tick, and a nil embed panics on the first call")
		Expect(instBD.Pointer()).To(Equal(reflect.ValueOf(frameworkBD).Pointer()),
			"newDeps kept the BaseDependencies the framework handed it, so Poll's logger is the framework's own rather than one built from a package global")
	})

	It("hands every instance built through the factory the same pool holder", func() {
		// Asserting on newDeps alone pins the helper, not the registration: a spec
		// that calls newDeps and then overwrites pool with a fresh holder passes
		// that assertion while the goroutine leak is back. This one goes through
		// the path a spawning supervisor takes.
		a := instOf(boundDepsOf(idUnder("parent-a"))).FieldByName("pool")
		b := instOf(boundDepsOf(idUnder("parent-b"))).FieldByName("pool")

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
			newDeps(idUnder("parent-a"), baseUnder("parent-a")), cfg)

		Expect(err).To(MatchError(ContainSubstring("parse timescale dsn")),
			"the error wraps the parse failure, so the degraded verdict names the cause")
		Expect(err).NotTo(MatchError(ContainSubstring(cfg.Timescale.Password)),
			"the error quotes the DSN, which carries the password: pgx masks it, and this error is logged")
		Expect(status.Reachable).To(BeFalse(), "nothing was dialled, so the endpoint is not proven reachable")
		Expect(status.Auth).To(Equal(models.TimescaleAuthUnknown),
			"no server answered, so the credentials stay unverified rather than rejected")
	})

	It("reuses the pool cached by the holder it was handed", func() {
		// The two sharing specs pin where the holder is stored, not that Poll reads
		// it: a Poll that ignored d.pool and called (&poolHolder{}).get itself would
		// pass them while dialling a fresh pool, and leaking its health-check
		// goroutine, once per second. This one reads the pool back off the holder
		// Poll was handed, so it fails on either rewiring.
		//
		// A closed port keeps the spec offline. pgxpool.NewWithConfig does not dial
		// (MinConns and MinIdleConns both default to 0, so it builds the pool from
		// the parsed DSN alone); only the SELECT 1 reaches the network, where it is
		// refused at once.
		cfg := config.HistorianConfig{Timescale: config.TimescaleConfig{
			Host:     "127.0.0.1",
			Port:     closedPort(),
			Password: "unlikely-to-appear-by-accident",
			SSLMode:  config.HistorianSSLModeDisable,
		}}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		d := newDeps(idUnder("parent-a"), baseUnder("parent-a"))

		_, err := Poll(ctx, d, cfg)
		Expect(err).To(MatchError(ContainSubstring("timescale query")),
			"the DSN parsed and the pool was built, so the failure is the refused SELECT 1")

		first := pooledBy(d.pool)
		Expect(first).NotTo(BeNil(),
			"Poll went through the holder in its deps: a Poll that built its own pool leaves this nil")

		_, err = Poll(ctx, d, cfg)
		Expect(err).To(HaveOccurred(), "still nothing listening on the port")

		Expect(pooledBy(d.pool)).To(BeIdenticalTo(first),
			"the second poll reused the cached pool rather than building a second one")
	})
})
