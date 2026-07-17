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
