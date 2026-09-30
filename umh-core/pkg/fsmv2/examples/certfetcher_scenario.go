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

// Subscribers returns mock subscriber emails via the subscriber handler.
func (m *MockCertHandler) Subscribers() []string {
	m.mu.RLock()
	sh := m.subHandler
	m.mu.RUnlock()
	if sh == nil {
		return nil
	}
	return sh.Subscribers()
}

// HasSubHandler returns true when the mock subscriber handler is set.
func (m *MockCertHandler) HasSubHandler() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.subHandler != nil
}

// SetSubHandler satisfies certificatehandler.Handler; the mock seeds its
// subscriber handler at construction, so this is a no-op.
func (m *MockCertHandler) SetSubHandler(_ certificatehandler.SubHandler) {}

// FetchCallCount returns how many times FetchAllCerts was called.
func (m *MockCertHandler) FetchCallCount() int {
	return int(m.fetchCount.Load())
}

var errCertFetchSimulated = errors.New("simulated cert fetch failure")

func certFetcherDependencies(emails []string, fetchErr error) (map[string]any, func(), error) {
	handler := NewMockCertHandler(emails, fetchErr)

	deps := map[string]any{}

	// Declared as the interface: a *MockCertHandler argument does not match
	// the key's type, so SetDependency would not compile.
	var h certificatehandler.Handler = handler

	config.SetDependency(deps, certfetcher.CertHandlerKey, h)

	return deps, nil, nil
}

// upsertCertFetcher creates the certfetcher worker. CertFetcherConfig has no
// fields, so the state key has no effect.
func upsertCertFetcher(env Env) error {
	ref := dynamicchildren.Ref{WorkerType: certfetcher.WorkerTypeName, Name: "certfetcher-1"}

	return env.Client.Upsert(ref, map[string]any{"state": "running"})
}

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
// a subscriber and whose fetch succeeds.
var CertFetcherHealthyScenarioV2 = ScenarioV2{
	Name:        "certfetcher-healthy",
	Description: "Cert fetcher with a subscriber: reaches Running and fetches",

	Dependencies: func() (map[string]any, func(), error) {
		return certFetcherDependencies([]string{"alice@example.com"}, nil)
	},

	Run: func(ctx context.Context, env Env) error {
		ref := dynamicchildren.Ref{WorkerType: certfetcher.WorkerTypeName, Name: "certfetcher-1"}

		env.Step("create certfetcher; its mock handler lists one subscriber, alice@example.com")

		if err := upsertCertFetcher(env); err != nil {
			return err
		}

		if err := waitForCertFetcherState(ctx, env, ref, "Running"); err != nil {
			return err
		}

		// A successful fetch sets LastFetchAt (RecordFetchSuccess in the
		// worker's dependencies), so a non-zero value shows the worker
		// fetched through the handler.
		return env.WaitFor(ctx, "store shows a successful fetch (last_fetch_at is set)",
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
// has a subscriber but fails every fetch. Each failure logs an action_failed
// error, which ExpectedErrorCauses allows. After DegradedThreshold
// (certfetcher/state/state_running.go) failed fetches in a row, the worker
// moves Running -> Degraded.
var CertFetcherDegradedScenarioV2 = ScenarioV2{
	Name:        "certfetcher-degraded",
	Description: "Cert fetcher whose fetches fail: enters Degraded after DegradedThreshold (certfetcher/state) failed fetches in a row",

	ExpectedErrorCauses: []error{errCertFetchSimulated},

	Dependencies: func() (map[string]any, func(), error) {
		return certFetcherDependencies([]string{"alice@example.com"}, errCertFetchSimulated)
	},

	Run: func(ctx context.Context, env Env) error {
		ref := dynamicchildren.Ref{WorkerType: certfetcher.WorkerTypeName, Name: "certfetcher-1"}

		env.Step("create certfetcher whose every fetch fails; each failure logs an expected action_failed error")

		if err := upsertCertFetcher(env); err != nil {
			return err
		}

		return waitForCertFetcherState(ctx, env, ref, "Degraded")
	},
}

// CertFetcherNoSubscribersScenarioV2 runs one certfetcher worker whose cert
// handler has no subscriber handler (the SubHandler that lists active
// subscribers). StoppedState starts the worker only once a subscriber handler
// exists (certfetcher/state/state_stopped.go). So the worker stays Stopped, and
// the log shows no state_transition line for it. The scenario passes when the
// worker is still Stopped after 20 polls.
var CertFetcherNoSubscribersScenarioV2 = ScenarioV2{
	Name:        "certfetcher-no-subscribers",
	Description: "Cert fetcher with no subscriber handler: stays in Stopped",

	Dependencies: func() (map[string]any, func(), error) {
		return certFetcherDependencies(nil, nil)
	},

	Run: func(ctx context.Context, env Env) error {
		ref := dynamicchildren.Ref{WorkerType: certfetcher.WorkerTypeName, Name: "certfetcher-1"}

		env.Step("create certfetcher with no subscriber handler; it stays Stopped, so no state change appears for it")

		if err := upsertCertFetcher(env); err != nil {
			return err
		}

		polls := 0

		return env.WaitFor(ctx, "the certfetcher is still Stopped after 20 polls",
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
