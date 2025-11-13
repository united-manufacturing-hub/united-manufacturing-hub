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

package storage_test

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

type mockTriangularStore struct {
	SaveIdentityErr error
	LoadIdentityErr error
	SaveDesiredErr  error
	LoadDesiredErr  error
	SaveObservedErr error
	LoadObservedErr error
	LoadSnapshotErr error

	identity map[string]map[string]persistence.Document
	desired  map[string]map[string]persistence.Document
	observed map[string]map[string]persistence.Document
	registry *storage.Registry

	SaveDesiredCalled  int
	LoadDesiredCalled  int
	SaveObservedCalled int
	LoadObservedCalled int
}

func NewMockTriangularStore() *mockTriangularStore {
	return &mockTriangularStore{
		identity: make(map[string]map[string]persistence.Document),
		desired:  make(map[string]map[string]persistence.Document),
		observed: make(map[string]map[string]persistence.Document),
		registry: storage.NewRegistry(),
	}
}

func (m *mockTriangularStore) SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error {
	if m.SaveIdentityErr != nil {
		return m.SaveIdentityErr
	}

	if m.identity[workerType] == nil {
		m.identity[workerType] = make(map[string]persistence.Document)
	}

	m.identity[workerType][id] = identity

	return nil
}

func (m *mockTriangularStore) LoadIdentity(ctx context.Context, workerType string, id string) (persistence.Document, error) {
	if m.LoadIdentityErr != nil {
		return nil, m.LoadIdentityErr
	}

	if m.identity[workerType] == nil {
		return nil, persistence.ErrNotFound
	}

	doc, ok := m.identity[workerType][id]
	if !ok {
		return nil, persistence.ErrNotFound
	}

	return doc, nil
}

func (m *mockTriangularStore) SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) error {
	m.SaveDesiredCalled++

	if m.SaveDesiredErr != nil {
		return m.SaveDesiredErr
	}

	if m.desired[workerType] == nil {
		m.desired[workerType] = make(map[string]persistence.Document)
	}

	m.desired[workerType][id] = desired

	return nil
}

func (m *mockTriangularStore) LoadDesired(ctx context.Context, workerType string, id string) (interface{}, error) {
	m.LoadDesiredCalled++

	if m.LoadDesiredErr != nil {
		return nil, m.LoadDesiredErr
	}

	if m.desired[workerType] == nil {
		return nil, persistence.ErrNotFound
	}

	doc, ok := m.desired[workerType][id]
	if !ok {
		return nil, persistence.ErrNotFound
	}

	return doc, nil
}

func (m *mockTriangularStore) SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (bool, error) {
	m.SaveObservedCalled++

	if m.SaveObservedErr != nil {
		return false, m.SaveObservedErr
	}

	if m.observed[workerType] == nil {
		m.observed[workerType] = make(map[string]persistence.Document)
	}

	var doc persistence.Document
	if observedDoc, ok := observed.(persistence.Document); ok {
		doc = observedDoc
	} else {
		doc = persistence.Document{"data": observed}
	}

	m.observed[workerType][id] = doc

	return true, nil
}

func (m *mockTriangularStore) LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error) {
	m.LoadObservedCalled++

	if m.LoadObservedErr != nil {
		return nil, m.LoadObservedErr
	}

	if m.observed[workerType] == nil {
		return nil, persistence.ErrNotFound
	}

	doc, ok := m.observed[workerType][id]
	if !ok {
		return nil, persistence.ErrNotFound
	}

	return doc, nil
}

func (m *mockTriangularStore) LoadSnapshot(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {
	if m.LoadSnapshotErr != nil {
		return nil, m.LoadSnapshotErr
	}

	snapshot := &storage.Snapshot{}

	if idMap, ok := m.identity[workerType]; ok {
		snapshot.Identity = idMap[id]
	}

	if desMap, ok := m.desired[workerType]; ok {
		snapshot.Desired = desMap[id]
	}

	if obsMap, ok := m.observed[workerType]; ok {
		snapshot.Observed = obsMap[id]
	}

	return snapshot, nil
}

func (m *mockTriangularStore) Registry() *storage.Registry {
	return m.registry
}

var _ storage.TriangularStoreInterface = (*mockTriangularStore)(nil)
