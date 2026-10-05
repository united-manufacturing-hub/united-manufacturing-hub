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

// Package fsmv2datacontract is a read-only monitor built on the fsmv2 simple
// framework. Each tick it compares the subjects the config expects with the
// subjects the Redpanda Schema Registry holds, and reports the ones that are
// missing or could not be translated, so a contract that is saved but not
// enforced shows up as degraded. Registering the schemas stays with the FSMv1
// SchemaRegistry.Reconcile; this worker never writes to the registry.
//
// A subject counts as missing only after missingGrace, which covers the gap
// between a save and the next reconcile and a registry restart.
package fsmv2datacontract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
)

const (
	// WorkerType is the canonical worker-type name used in config and CSE storage.
	WorkerType = "datacontract"

	// InstanceName is the fixed dynamic-child name for the single monitor.
	InstanceName = "datacontract"

	// ConfigManagerDepsKey is the register.SetDeps key holding the
	// config.ConfigManager Poll reads. It is not configworker's key because
	// configworker imports this package.
	ConfigManagerDepsKey = WorkerType + ".configmanager"

	// PollInterval is the poll cadence; readers call an observation stale at
	// three times it.
	PollInterval = 2 * time.Second

	// missingGrace is how long a subject may be absent before it is reported.
	missingGrace = 10 * time.Second
)

// Ref is the pair the configworker upserts this child under and the status
// generator reads it back with.
var Ref = dynamicchildren.Ref{WorkerType: WorkerType, Name: InstanceName}

var httpClient = &http.Client{Timeout: PollInterval}

// Status lists what the registry does not enforce. Both lists are empty when
// every expected schema is registered.
type Status struct {
	// Missing holds the expected subjects, such as "_pump_v1-timeseries-number",
	// that the registry has not held for missingGrace.
	Missing []string `json:"missing,omitempty"`
	// Untranslated holds the subject prefixes, such as "_pump_v1-", of the
	// contracts that could not be translated into a schema.
	Untranslated []string `json:"untranslated,omitempty"`
}

// Deps is the per-instance state Poll keeps across ticks; it is a pointer
// because Poll receives TDeps by value.
type Deps struct {
	*deps.BaseDependencies

	configManager config.ConfigManager
	translator    *redpanda.SchemaRegistry
	subjectsURL   string
	missingSince  map[string]time.Time
	lastOK        time.Time
	last          Status
}

func newDeps(_ deps.Identity, bd *deps.BaseDependencies) *Deps {
	return &Deps{
		BaseDependencies: bd,
		configManager:    register.GetDeps[config.ConfigManager](ConfigManagerDepsKey),
		translator:       redpanda.NewSchemaRegistry(),
		subjectsURL:      redpanda.DefaultSchemaRegistryAddress + "/subjects",
		lastOK:           time.Now(),
	}
}

// Poll reads the config and the registry once. A failed read keeps the last
// status for missingGrace before it degrades the worker. A nil subject map
// means ctx was cancelled.
func Poll(ctx context.Context, d *Deps, _ struct{}) (Status, error) {
	cfg, err := d.configManager.GetConfig(ctx, 0)
	if err != nil {
		return d.hold(fmt.Errorf("read config: %w", err))
	}

	expected, untranslated := d.translator.ExpectedSubjects(ctx, cfg)
	if expected == nil {
		return Status{}, ctx.Err()
	}

	var registered map[string]bool
	if len(expected) > 0 {
		registered, err = fetchSubjects(ctx, d.subjectsURL)
		if err != nil {
			return d.hold(err)
		}
	}

	now := time.Now()
	status := Status{Untranslated: untranslated}
	missingSince := map[string]time.Time{}

	for subject := range expected {
		name := string(subject)
		if registered[name] {
			continue
		}

		since, seen := d.missingSince[name]
		if !seen {
			since = now
		}

		missingSince[name] = since
		if now.Sub(since) >= missingGrace {
			status.Missing = append(status.Missing, name)
		}
	}

	slices.Sort(status.Missing)
	d.missingSince, d.lastOK, d.last = missingSince, now, status

	return status, nil
}

func (d *Deps) hold(err error) (Status, error) {
	if time.Since(d.lastOK) < missingGrace {
		return d.last, nil
	}

	return Status{}, err
}

func fetchSubjects(ctx context.Context, url string) (map[string]bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("schema registry: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("schema registry: list subjects returned %s", resp.Status)
	}

	var subjects []string
	if err := json.NewDecoder(resp.Body).Decode(&subjects); err != nil {
		return nil, fmt.Errorf("schema registry: decode subjects: %w", err)
	}

	registered := make(map[string]bool, len(subjects))
	for _, subject := range subjects {
		registered[subject] = true
	}

	return registered, nil
}

func init() {
	simple.Register(simple.MonitorSpec[struct{}, Status, *Deps]{
		WorkerType: WorkerType,
		Interval:   PollInterval,
		Poll:       Poll,
		NewDeps:    newDeps,
		Health: func(_ struct{}, s Status) simple.Health {
			if len(s.Missing) == 0 && len(s.Untranslated) == 0 {
				return simple.Healthy("every data contract schema is registered")
			}

			return simple.Degraded(fmt.Sprintf("%d subjects not registered, %d contracts not translated", len(s.Missing), len(s.Untranslated)))
		},
	})
}
