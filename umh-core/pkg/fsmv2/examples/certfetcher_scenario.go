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
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
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

// FetchCallCount returns how many times FetchAllCerts was called.
func (m *MockCertHandler) FetchCallCount() int {
	return int(m.fetchCount.Load())
}

// CertFetcherRunConfig configures a cert fetcher scenario run.
type CertFetcherRunConfig struct {
	SubscriberEmails []string      // Emails the mock sub handler returns; nil means no sub handler
	FetchError       error         // If set, FetchAllCerts returns this error
	Duration         time.Duration // How long to run the scenario
	TickInterval     time.Duration // Defaults to 100ms
	Logger           deps.FSMLogger
}

// CertFetcherRunResult contains observable results after scenario completion.
type CertFetcherRunResult struct {
	Error          error
	Done           <-chan struct{}
	Shutdown       func()
	FetchCallCount int
}

// RunCertFetcherScenario runs the FSMv2 certfetcher worker via ApplicationSupervisor with a mock cert handler.
func RunCertFetcherScenario(ctx context.Context, cfg CertFetcherRunConfig) *CertFetcherRunResult {
	done := make(chan struct{})

	if cfg.Duration < 0 {
		close(done)

		return &CertFetcherRunResult{
			Done:     done,
			Shutdown: func() {},
			Error:    fmt.Errorf("invalid duration %v: must be non-negative", cfg.Duration),
		}
	}

	if ctx.Err() != nil {
		close(done)

		return &CertFetcherRunResult{
			Done:     done,
			Shutdown: func() {},
			Error:    fmt.Errorf("context already cancelled: %w", ctx.Err()),
		}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = deps.NewNopFSMLogger()
	}

	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = 100 * time.Millisecond
	}

	mockHandler := NewMockCertHandler(cfg.SubscriberEmails, cfg.FetchError)

	store := SetupStore(logger)

	yamlConfig := `
children:
  - name: "certfetcher"
    workerType: "certfetcher"
`

	appSup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:           "scenario-certfetcher",
		Name:         "certfetcher",
		Store:        store,
		Logger:       logger,
		TickInterval: tickInterval,
		YAMLConfig:   yamlConfig,
		Dependencies: map[string]any{
			"certHandler": mockHandler,
		},
	})
	if err != nil {
		close(done)

		return &CertFetcherRunResult{
			Done:     done,
			Shutdown: func() {},
			Error:    fmt.Errorf("failed to create supervisor: %w", err),
		}
	}

	supDone := appSup.Start(ctx)

	result := &CertFetcherRunResult{
		Done:     done,
		Shutdown: appSup.Shutdown,
	}

	go func() {
		if cfg.Duration > 0 {
			select {
			case <-time.After(cfg.Duration):
				appSup.Shutdown()
			case <-ctx.Done():
				appSup.Shutdown()
			case <-supDone:
			}
		} else {
			select {
			case <-ctx.Done():
				appSup.Shutdown()
			case <-supDone:
			}
		}

		<-supDone

		result.FetchCallCount = mockHandler.FetchCallCount()

		close(done)
	}()

	return result
}
