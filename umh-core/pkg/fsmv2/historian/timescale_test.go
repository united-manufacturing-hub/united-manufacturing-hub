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
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		err          error
		name         string
		wantAnswered bool
		wantAuth     bool
	}{
		{
			name:         "auth: invalid password (28P01)",
			err:          &pgconn.PgError{Code: "28P01", Message: "password authentication failed"},
			wantAnswered: true,
			wantAuth:     true,
		},
		{
			name:         "auth: invalid authorization class (28000)",
			err:          &pgconn.PgError{Code: "28000", Message: "invalid authorization specification"},
			wantAnswered: true,
			wantAuth:     true,
		},
		{
			name:         "unknown database: postgres invalid catalog (3D000)",
			err:          &pgconn.PgError{Code: "3D000", Message: `database "nope" does not exist`},
			wantAnswered: true,
			wantAuth:     true,
		},
		{
			name:         "unknown database: pgbouncer missing database (08P01)",
			err:          &pgconn.PgError{Code: "08P01", Message: "no such database: nope"},
			wantAnswered: true,
			wantAuth:     true,
		},
		{
			name:         "protocol violation: unrelated 08P01 reachable but not auth",
			err:          &pgconn.PgError{Code: "08P01", Message: "invalid startup packet"},
			wantAnswered: true,
			wantAuth:     false,
		},
		{
			name:         "server error: syntax error (42601) reachable but not auth",
			err:          &pgconn.PgError{Code: "42601", Message: "syntax error"},
			wantAnswered: true,
			wantAuth:     false,
		},
		{
			name:         "timeout: context deadline exceeded unreachable",
			err:          context.DeadlineExceeded,
			wantAnswered: false,
			wantAuth:     false,
		},
		{
			name:         "network: plain error unreachable",
			err:          errors.New("dial tcp: connection refused"),
			wantAnswered: false,
			wantAuth:     false,
		},
		{
			name:         "wrapped: pgbouncer missing database through fmt.Errorf",
			err:          fmt.Errorf("timescale query: %w", &pgconn.PgError{Code: "08P01", Message: "no such database: nope"}),
			wantAnswered: true,
			wantAuth:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serverAnswered(tt.err); got != tt.wantAnswered {
				t.Errorf("serverAnswered() = %v, want %v", got, tt.wantAnswered)
			}
			if got := authRejected(tt.err); got != tt.wantAuth {
				t.Errorf("authRejected() = %v, want %v", got, tt.wantAuth)
			}
		})
	}
}
