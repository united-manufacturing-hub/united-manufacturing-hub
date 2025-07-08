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

package redpanda

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type SchemaRegistry struct {
	currentPhase SchemaRegistryPhase
	httpClient   http.Client
	subjects     []string
}

type SchemaRegistryPhase string

const (
	SchemaRegistryPhaseLookup  SchemaRegistryPhase = "lookup"
	SchemaRegistryPhaseCompare SchemaRegistryPhase = "compare"
	SchemaRegistryPhaseApply   SchemaRegistryPhase = "apply"
)

const SchemaRegistryAddress = "localhost:8081"
const MinimumLookupTime = 20 * time.Millisecond

func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		currentPhase: SchemaRegistryPhaseLookup,
		httpClient:   http.Client{},
		subjects:     make([]string, 0),
	}
}

func (s *SchemaRegistry) Reconcile(ctx context.Context) (err error, reconciled bool) {
	switch s.currentPhase {
	case SchemaRegistryPhaseLookup:
		return s.lookup(ctx)
	case SchemaRegistryPhaseCompare:
		return s.compare(ctx)
	case SchemaRegistryPhaseApply:
		return s.apply(ctx)
	default:
		return fmt.Errorf("unknown phase: %s", s.currentPhase), false
	}
}

func (s *SchemaRegistry) lookup(ctx context.Context) (err error, reconciled bool) {
	// Check if context has enough time remaining
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) < MinimumLookupTime {
			return fmt.Errorf("insufficient time remaining in context (< %v)", MinimumLookupTime), false
		}
	}

	url := fmt.Sprintf("http://%s/subjects", SchemaRegistryAddress)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err, false
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("schema registry returned status %d", resp.StatusCode), false
	}

	var subjects []string
	if err := json.NewDecoder(resp.Body).Decode(&subjects); err != nil {
		return err, false
	}

	s.subjects = subjects
	s.currentPhase = SchemaRegistryPhaseCompare

	return nil, true
}

func (s *SchemaRegistry) compare(ctx context.Context) (err error, reconciled bool) {
	// TODO: Implement
	return nil, false
}

func (s *SchemaRegistry) apply(ctx context.Context) (err error, reconciled bool) {
	// TODO: Implement
	return nil, false
}
