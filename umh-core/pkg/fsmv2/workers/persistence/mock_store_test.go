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

package persistence_test

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

type mockTriangularStore struct {
	compactDeltasResult int
	compactDeltasErr    error

	maintenanceErr error
}

var _ storage.TriangularStoreInterface = (*mockTriangularStore)(nil)

func (m *mockTriangularStore) CompactDeltas(_ context.Context, _ time.Duration) (int, error) {
	return m.compactDeltasResult, m.compactDeltasErr
}

func (m *mockTriangularStore) Maintenance(_ context.Context) error {
	return m.maintenanceErr
}

func (m *mockTriangularStore) SaveIdentity(_ context.Context, _ string, _ string, _ persistence.Document) error {
	panic("not implemented")
}

func (m *mockTriangularStore) LoadIdentity(_ context.Context, _ string, _ string) (persistence.Document, error) {
	panic("not implemented")
}

func (m *mockTriangularStore) SaveDesired(_ context.Context, _ string, _ string, _ persistence.Document) (bool, error) {
	panic("not implemented")
}

func (m *mockTriangularStore) LoadDesired(_ context.Context, _ string, _ string) (interface{}, error) {
	panic("not implemented")
}

func (m *mockTriangularStore) LoadDesiredTyped(_ context.Context, _ string, _ string, _ interface{}) error {
	panic("not implemented")
}

func (m *mockTriangularStore) SaveObserved(_ context.Context, _ string, _ string, _ interface{}) (bool, error) {
	panic("not implemented")
}

func (m *mockTriangularStore) LoadObserved(_ context.Context, _ string, _ string) (interface{}, error) {
	panic("not implemented")
}

func (m *mockTriangularStore) LoadObservedTyped(_ context.Context, _ string, _ string, _ interface{}) error {
	panic("not implemented")
}

func (m *mockTriangularStore) LoadSnapshot(_ context.Context, _ string, _ string) (*storage.Snapshot, error) {
	panic("not implemented")
}

func (m *mockTriangularStore) GetLatestSyncID(_ context.Context) (int64, error) {
	panic("not implemented")
}

func (m *mockTriangularStore) GetDeltas(_ context.Context, _ storage.Subscription) (storage.DeltasResponse, error) {
	panic("not implemented")
}
