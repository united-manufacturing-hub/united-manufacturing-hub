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

package examples

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	certfetcher "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
)

var _ certificatehandler.Handler = (*MockCertHandler)(nil)

// MockSubHandler implements certificatehandler.SubHandler for tests.
type MockSubHandler struct {
	emails []string
}

// Subscribers returns the configured subscriber emails.
func (m *MockSubHandler) Subscribers() []string {
	return m.emails
}

// MockCertHandler implements certificatehandler.Handler with configurable behavior.
type MockCertHandler struct {
	subHandler *MockSubHandler
	fetchError error
	fetchCount atomic.Int64
	certs      map[string]*x509.Certificate

	mu sync.RWMutex
}

// NewMockCertHandler creates a mock cert handler.
func NewMockCertHandler(emails []string, fetchError error) *MockCertHandler {
	m := &MockCertHandler{
		fetchError: fetchError,
		certs:      make(map[string]*x509.Certificate),
	}

	if emails != nil {
		m.subHandler = &MockSubHandler{emails: emails}
		for _, email := range emails {
			m.certs[email] = &x509.Certificate{
				SerialNumber: big.NewInt(1),
				Subject:      pkix.Name{CommonName: email},
			}
		}
	}

	return m
}

// Certificate returns a dummy certificate for known emails.
func (m *MockCertHandler) Certificate(email string) *x509.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.certs[email]
}

// IntermediateCerts returns nil (not needed for tests).
func (m *MockCertHandler) IntermediateCerts(_ string) []*x509.Certificate {
	return nil
}

// RootCA returns nil (not needed for tests).
func (m *MockCertHandler) RootCA() *x509.Certificate {
	return nil
}

// FetchCertForEmail fetches a single user's certificate on demand.
func (m *MockCertHandler) FetchCertForEmail(_ context.Context, _ string) error {
	return m.fetchError
}

// FetchAllCerts tracks call count and returns the configured error.
func (m *MockCertHandler) FetchAllCerts(_ context.Context) error {
	m.fetchCount.Add(1)
	return m.fetchError
}

// Subscribers returns mock subscriber emails via the sub handler.
func (m *MockCertHandler) Subscribers() []string {
	m.mu.RLock()
	sh := m.subHandler
	m.mu.RUnlock()
	if sh == nil {
		return nil
	}
	return sh.Subscribers()
}

// HasSubHandler returns true when the mock sub handler is set.
func (m *MockCertHandler) HasSubHandler() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.subHandler != nil
}

// SetSubHandler satisfies certificatehandler.Handler; the mock seeds its sub
// handler at construction, so this is a no-op.
func (m *MockCertHandler) SetSubHandler(_ certificatehandler.SubHandler) {}

// FetchCallCount returns how many times FetchAllCerts was called.
func (m *MockCertHandler) FetchCallCount() int {
	return int(m.fetchCount.Load())
}

// errCertFetchSimulated is the fetch error the degraded scenario's handler
// returns. The fetch action returns the handler's error unwrapped, so the
// executor's action_failed error carries this cause.
var errCertFetchSimulated = errors.New("simulated cert fetch failure")

// certFetcherDependencies builds the dependency map one certfetcher scenario
// runs against: a MockCertHandler with the given subscribers and fetch
// error, stored under the worker's CertHandlerKey. The var declaration keeps
// SetDependency's type parameter the Handler interface, not the mock.
func certFetcherDependencies(emails []string, fetchErr error) (map[string]any, func(), error) {
	handler := NewMockCertHandler(emails, fetchErr)

	deps := map[string]any{}

	var h certificatehandler.Handler = handler

	config.SetDependency(deps, certfetcher.CertHandlerKey, h)

	return deps, nil, nil
}

// upsertCertFetcher creates the certfetcher worker every scenario below
// runs. CertFetcherConfig has no fields, so the state key is accepted and
// ignored; it is sent anyway to keep the same shape as the other scenarios.
func upsertCertFetcher(env Env) error {
	ref := dynamicchildren.Ref{WorkerType: certfetcher.WorkerTypeName, Name: "certfetcher-1"}

	return env.Client.Upsert(ref, map[string]any{"state": "running"})
}

// waitForCertFetcherState waits until the certfetcher worker's stored
// observation reports the given state.
func waitForCertFetcherState(ctx context.Context, env Env, ref dynamicchildren.Ref, want string) error {
	return env.WaitFor(ctx, "store shows state "+want,
		func(ctx context.Context) (bool, string, error) {
			obs, err := fsmv2client.Get[certfetcher.CertFetcherStatus](ctx, env.Client, ref)
			if err != nil {
				if errors.Is(err, fsmv2client.ErrNotObserved) {
					return false, "the worker has not published an observation yet", nil
				}

				return false, "", err
			}

			return obs.State == want, "state=" + obs.State, nil
		})
}

// CertFetcherHealthyScenarioV2 runs one certfetcher worker whose handler has
// a subscriber and whose fetch succeeds: the worker reaches Running and
// completes a fetch.
var CertFetcherHealthyScenarioV2 = ScenarioV2{
	Name:        "certfetcher-healthy",
	Description: "Cert fetcher with a subscriber: reaches Running and fetches",

	Dependencies: func() (map[string]any, func(), error) {
		return certFetcherDependencies([]string{"alice@example.com"}, nil)
	},

	Run: func(ctx context.Context, env Env) error {
		ref := dynamicchildren.Ref{WorkerType: certfetcher.WorkerTypeName, Name: "certfetcher-1"}

		env.Step("create certfetcher with a subscriber handler")

		if err := upsertCertFetcher(env); err != nil {
			return err
		}

		if err := waitForCertFetcherState(ctx, env, ref, "Running"); err != nil {
			return err
		}

		// A successful fetch sets LastFetchAt (RecordFetchSuccess in the
		// worker's dependencies), so a non-zero value shows the worker
		// fetched through the handler.
		return env.WaitFor(ctx, "store shows a completed fetch",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[certfetcher.CertFetcherStatus](ctx, env.Client, ref)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				return !obs.Status.LastFetchAt.IsZero(),
					"last_fetch_at=" + obs.Status.LastFetchAt.Format(time.RFC3339), nil
			})
	},
}

// CertFetcherDegradedScenarioV2 runs one certfetcher worker whose handler
// has a subscriber but fails every fetch: three consecutive failures reach
// the error threshold and the worker enters Degraded.
var CertFetcherDegradedScenarioV2 = ScenarioV2{
	Name:        "certfetcher-degraded",
	Description: "Cert fetcher whose fetches fail: enters Degraded after the threshold",

	// Each failed fetch makes the executor log action_failed with the
	// handler's error, which carries errCertFetchSimulated.
	ExpectedErrorCauses: []error{errCertFetchSimulated},

	Dependencies: func() (map[string]any, func(), error) {
		return certFetcherDependencies([]string{"alice@example.com"}, errCertFetchSimulated)
	},

	Run: func(ctx context.Context, env Env) error {
		ref := dynamicchildren.Ref{WorkerType: certfetcher.WorkerTypeName, Name: "certfetcher-1"}

		env.Step("create certfetcher with a subscriber handler and a failing fetch")

		if err := upsertCertFetcher(env); err != nil {
			return err
		}

		// Three consecutive failed fetches reach DegradedThreshold
		// (the worker's RunningState).
		return waitForCertFetcherState(ctx, env, ref, "Degraded")
	},
}

// CertFetcherNoSubscribersScenarioV2 runs one certfetcher worker whose
// handler has no sub handler, so the worker never leaves Stopped.
var CertFetcherNoSubscribersScenarioV2 = ScenarioV2{
	Name:        "certfetcher-no-subscribers",
	Description: "Cert fetcher without a subscriber handler: stays in Stopped",

	Dependencies: func() (map[string]any, func(), error) {
		return certFetcherDependencies(nil, nil)
	},

	Run: func(ctx context.Context, env Env) error {
		ref := dynamicchildren.Ref{WorkerType: certfetcher.WorkerTypeName, Name: "certfetcher-1"}

		env.Step("create certfetcher without a subscriber handler")

		if err := upsertCertFetcher(env); err != nil {
			return err
		}

		// StoppedState leaves Stopped only when the handler has a sub
		// handler (the worker's state_stopped.go), so a worker whose handler
		// has none stays in Stopped by construction. The wait samples it: a
		// single observation in any other state fails at once.
		polls := 0

		return env.WaitFor(ctx, "the worker stays in Stopped across 20 polls",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[certfetcher.CertFetcherStatus](ctx, env.Client, ref)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				if obs.State != "Stopped" {
					return false, "", fmt.Errorf("the worker reached %s, so it left Stopped without a subscriber handler", obs.State)
				}

				polls++

				return polls >= 20, fmt.Sprintf("state=%s polls=%d", obs.State, polls), nil
			})
	},
}
