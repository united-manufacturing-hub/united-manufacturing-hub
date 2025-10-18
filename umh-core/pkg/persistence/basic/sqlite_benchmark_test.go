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

package basic_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

func BenchmarkInsert(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}

	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err = store.CreateCollection(ctx, "bench_insert", nil)
	if err != nil {
		b.Fatalf("Failed to create collection: %v", err)
	}

	b.ResetTimer()

	for i := range b.N {
		doc := basic.Document{
			"index": i,
			"value": "benchmark_data",
		}

		_, err := store.Insert(ctx, "bench_insert", doc)
		if err != nil {
			b.Fatalf("Insert failed: %v", err)
		}
	}
}

func BenchmarkGet(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}

	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err = store.CreateCollection(ctx, "bench_get", nil)
	if err != nil {
		b.Fatalf("Failed to create collection: %v", err)
	}

	ids := make([]string, 1000)

	for i := range 1000 {
		doc := basic.Document{
			"index": i,
			"value": "benchmark_data",
		}

		id, err := store.Insert(ctx, "bench_get", doc)
		if err != nil {
			b.Fatalf("Insert failed: %v", err)
		}

		ids[i] = id
	}

	b.ResetTimer()

	for i := range b.N {
		_, err := store.Get(ctx, "bench_get", ids[i%1000])
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

func BenchmarkUpdate(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}

	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err = store.CreateCollection(ctx, "bench_update", nil)
	if err != nil {
		b.Fatalf("Failed to create collection: %v", err)
	}

	ids := make([]string, 100)

	for i := range 100 {
		doc := basic.Document{
			"index": i,
			"value": "original_data",
		}

		id, err := store.Insert(ctx, "bench_update", doc)
		if err != nil {
			b.Fatalf("Insert failed: %v", err)
		}

		ids[i] = id
	}

	b.ResetTimer()

	for i := range b.N {
		doc := basic.Document{
			"index": i,
			"value": "updated_data",
		}

		err := store.Update(ctx, "bench_update", ids[i%100], doc)
		if err != nil {
			b.Fatalf("Update failed: %v", err)
		}
	}
}

func BenchmarkDelete(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}

	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err = store.CreateCollection(ctx, "bench_delete", nil)
	if err != nil {
		b.Fatalf("Failed to create collection: %v", err)
	}

	ids := make([]string, b.N)
	for i := range b.N {
		doc := basic.Document{
			"index": i,
			"value": "benchmark_data",
		}

		id, err := store.Insert(ctx, "bench_delete", doc)
		if err != nil {
			b.Fatalf("Insert failed: %v", err)
		}

		ids[i] = id
	}

	b.ResetTimer()

	for i := range b.N {
		err := store.Delete(ctx, "bench_delete", ids[i])
		if err != nil {
			b.Fatalf("Delete failed: %v", err)
		}
	}
}

func BenchmarkFind(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}

	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err = store.CreateCollection(ctx, "bench_find", nil)
	if err != nil {
		b.Fatalf("Failed to create collection: %v", err)
	}

	for i := range 1000 {
		doc := basic.Document{
			"index":    i,
			"value":    "benchmark_data",
			"category": i % 10,
		}

		_, err := store.Insert(ctx, "bench_find", doc)
		if err != nil {
			b.Fatalf("Insert failed: %v", err)
		}
	}

	b.ResetTimer()

	for i := range b.N {
		_, err := store.Find(ctx, "bench_find", basic.Query{
			Filters: []basic.FilterCondition{
				{Field: "category", Op: basic.Eq, Value: i % 10},
			},
			LimitCount: 100,
		})
		if err != nil {
			b.Fatalf("Find failed: %v", err)
		}
	}
}

func BenchmarkFindWithSort(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "bench.db")

	store, err := basic.NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}

	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err = store.CreateCollection(ctx, "bench_find_sort", nil)
	if err != nil {
		b.Fatalf("Failed to create collection: %v", err)
	}

	for i := range 1000 {
		doc := basic.Document{
			"index": i,
			"value": "benchmark_data",
		}

		_, err := store.Insert(ctx, "bench_find_sort", doc)
		if err != nil {
			b.Fatalf("Insert failed: %v", err)
		}
	}

	b.ResetTimer()

	for range b.N {
		_, err := store.Find(ctx, "bench_find_sort", basic.Query{
			SortBy: []basic.SortField{
				{Field: "index", Order: basic.Desc},
			},
			LimitCount: 100,
		})
		if err != nil {
			b.Fatalf("Find failed: %v", err)
		}
	}
}
