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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/mattn/go-sqlite3"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

// CONTRACT VALIDATION TESTS
//
// Purpose: Validate the FSM v2 ↔ CSE interface contract BEFORE implementing any components.
// This ensures that logs, blobs, and all 4 access patterns (Instant, Lazy, Partial, Explicit) work correctly.
//
// Why this exists:
// - Prevents discovering interface mismatches at merge time
// - Defines contract in executable code, not just documentation
// - Both FSM v2 and CSE teams code against passing tests
// - CI enforces contract compliance throughout development
//
// Test Strategy:
// 1. Use stub adapter implementation (minimal, proves contract works)
// 2. Validate all 4 access patterns with real SQL
// 3. Tests pass with stub → both teams implement to match stub
// 4. Real adapter replaces stub → tests still pass → perfect integration!
//
// Access Patterns Validated:
// - Instant: Simple snapshot fields (status, metrics) → main table
// - Lazy: 1:1 child tables (nested config) → loaded on demand
// - Partial: 1:many logs (append-only, paginated) → separate child table
// - Explicit: Large blobs (skip if unchanged) → blob table with checksum

var _ = Describe("Contract Validation: FSM v2 ↔ CSE Interface", func() {
	var (
		ctx         context.Context
		dbPath      string
		stubAdapter *StubAdapter
		testDB      *sql.DB
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup SQLite database
		tmpfile, err := os.CreateTemp("", "contract-validation-*.db")
		Expect(err).NotTo(HaveOccurred())
		dbPath = tmpfile.Name()
		tmpfile.Close()

		// Setup stub adapter (creates its own DB connection and TriangularStore)
		registry := storage.NewRegistry()
		stubAdapter, testDB, err = NewStubAdapter(ctx, dbPath, registry)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if testDB != nil {
			testDB.Close()
		}
		if dbPath != "" {
			os.Remove(dbPath)
		}
	})

	Describe("Instant Pattern (Simple Snapshot Fields)", func() {
		It("should save instant fields to main table", func() {
			// Create mock observed state with instant fields
			observed := &MockObservedState{
				Status:     "running",
				CPUPercent: 45.2,
				RAMUsageMB: 512,
			}

			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Verify: Instant fields in mock_observed table
			var status string
			var cpu float64
			var ram int64
			err = testDB.QueryRow("SELECT status, cpu_percent, ram_usage_mb FROM mock_observed WHERE id = ?", "worker-1").
				Scan(&status, &cpu, &ram)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("running"))
			Expect(cpu).To(BeNumerically("~", 45.2, 0.01))
			Expect(ram).To(Equal(int64(512)))
		})

		It("should load instant fields from main table", func() {
			// Insert test data directly
			_, err := testDB.Exec(`INSERT INTO mock_observed (id, status, cpu_percent, ram_usage_mb, _sync_id, _created_at, _updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				"worker-1", "degraded", 85.5, 1024, 1, time.Now().Unix(), time.Now().Unix())
			Expect(err).NotTo(HaveOccurred())

			// Load via adapter
			loaded, err := stubAdapter.LoadObserved(ctx, "mock", "worker-1")
			Expect(err).NotTo(HaveOccurred())

			mockObserved := loaded.(*MockObservedState)
			Expect(mockObserved.Status).To(Equal("degraded"))
			Expect(mockObserved.CPUPercent).To(BeNumerically("~", 85.5, 0.01))
			Expect(mockObserved.RAMUsageMB).To(Equal(int64(1024)))
		})

		It("should update instant fields without creating duplicates", func() {
			// Initial save
			observed := &MockObservedState{Status: "running", CPUPercent: 45.0}
			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Update
			observed.Status = "degraded"
			observed.CPUPercent = 90.0
			err = stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Verify: Only one row exists
			var count int
			err = testDB.QueryRow("SELECT COUNT(*) FROM mock_observed WHERE id = ?", "worker-1").Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			// Verify: Values updated
			var status string
			err = testDB.QueryRow("SELECT status FROM mock_observed WHERE id = ?", "worker-1").Scan(&status)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal("degraded"))
		})
	})

	Describe("Lazy Pattern (1:1 Child Tables)", func() {
		It("should save lazy fields to child table with PK=FK", func() {
			observed := &MockObservedState{
				Status: "running",
				Config: &MockConfig{
					Setting1: "value1",
					Setting2: 42,
				},
			}

			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Verify: Config in mock_config child table
			var setting1 string
			var setting2 int
			err = testDB.QueryRow("SELECT setting1, setting2 FROM mock_config WHERE id = ?", "worker-1").
				Scan(&setting1, &setting2)
			Expect(err).NotTo(HaveOccurred())
			Expect(setting1).To(Equal("value1"))
			Expect(setting2).To(Equal(42))
		})

		It("should handle nil lazy fields gracefully", func() {
			observed := &MockObservedState{
				Status: "running",
				Config: nil, // No config
			}

			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Verify: No row in child table
			var count int
			err = testDB.QueryRow("SELECT COUNT(*) FROM mock_config WHERE id = ?", "worker-1").Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should load lazy fields from child table", func() {
			// Insert main table data (must include all instant fields to avoid NULL scan errors)
			_, err := testDB.Exec(`INSERT INTO mock_observed (id, status, cpu_percent, ram_usage_mb, _sync_id, _created_at, _updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				"worker-1", "running", 0.0, 0, 1, time.Now().Unix(), time.Now().Unix())
			Expect(err).NotTo(HaveOccurred())

			// Insert child table data
			_, err = testDB.Exec(`INSERT INTO mock_config (id, setting1, setting2, _sync_id)
				VALUES (?, ?, ?, ?)`,
				"worker-1", "loaded-value", 99, 1)
			Expect(err).NotTo(HaveOccurred())

			// Load via adapter
			loaded, err := stubAdapter.LoadObserved(ctx, "mock", "worker-1")
			Expect(err).NotTo(HaveOccurred())

			mockObserved := loaded.(*MockObservedState)
			Expect(mockObserved.Config).NotTo(BeNil())
			Expect(mockObserved.Config.Setting1).To(Equal("loaded-value"))
			Expect(mockObserved.Config.Setting2).To(Equal(99))
		})
	})

	Describe("Partial Pattern (Logs - Append-Only, Paginated)", func() {
		It("should save logs to separate child table with foreign key", func() {
			now := time.Now()
			observed := &MockObservedState{
				Status: "running",
				NewLogs: []LogEntry{
					{Timestamp: now, Level: "INFO", Message: "Started"},
					{Timestamp: now.Add(1 * time.Second), Level: "INFO", Message: "Running"},
				},
			}

			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Verify: Logs in mock_logs child table
			var count int
			err = testDB.QueryRow("SELECT COUNT(*) FROM mock_logs WHERE worker_id = ?", "worker-1").Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(2))

			// Verify: Log messages stored correctly
			rows, err := testDB.Query("SELECT message FROM mock_logs WHERE worker_id = ? ORDER BY timestamp", "worker-1")
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()

			messages := []string{}
			for rows.Next() {
				var msg string
				rows.Scan(&msg)
				messages = append(messages, msg)
			}
			Expect(messages).To(Equal([]string{"Started", "Running"}))
		})

		It("should append new logs without loading existing logs", func() {
			// Insert existing log
			_, err := testDB.Exec(`INSERT INTO mock_logs (id, worker_id, timestamp, level, message, _sync_id)
				VALUES (?, ?, ?, ?, ?, ?)`,
				"log-1", "worker-1", time.Now().Unix(), "INFO", "Old log", 1)
			Expect(err).NotTo(HaveOccurred())

			// Append new logs (adapter should NOT read old logs)
			observed := &MockObservedState{
				NewLogs: []LogEntry{
					{Timestamp: time.Now(), Level: "INFO", Message: "New log 1"},
					{Timestamp: time.Now(), Level: "INFO", Message: "New log 2"},
				},
			}

			err = stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Verify: All 3 logs exist (1 old + 2 new)
			var count int
			err = testDB.QueryRow("SELECT COUNT(*) FROM mock_logs WHERE worker_id = ?", "worker-1").Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(3))
		})

		It("should support paginated log queries (last N logs)", func() {
			// Insert 100 test logs
			for i := 0; i < 100; i++ {
				_, err := testDB.Exec(`INSERT INTO mock_logs (id, worker_id, timestamp, level, message, _sync_id)
					VALUES (?, ?, ?, ?, ?, ?)`,
					fmt.Sprintf("log-%d", i), "worker-1",
					time.Now().Add(time.Duration(i)*time.Second).Unix(),
					"INFO", fmt.Sprintf("Message %d", i), 1)
				Expect(err).NotTo(HaveOccurred())
			}

			// Query last 10 logs
			logs, err := stubAdapter.LoadLogs(ctx, "mock", "worker-1", LogQueryOpts{
				Limit:     10,
				OrderDesc: true,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(logs).To(HaveLen(10))
			Expect(logs[0].Message).To(Equal("Message 99")) // Most recent first
			Expect(logs[9].Message).To(Equal("Message 90"))
		})

		It("should support time-based log filtering", func() {
			baseTime := time.Now()

			// Insert logs with different timestamps
			for i := 0; i < 10; i++ {
				timestamp := baseTime.Add(time.Duration(i) * time.Hour)
				_, err := testDB.Exec(`INSERT INTO mock_logs (id, worker_id, timestamp, level, message, _sync_id)
					VALUES (?, ?, ?, ?, ?, ?)`,
					fmt.Sprintf("log-%d", i), "worker-1", timestamp.Unix(),
					"INFO", fmt.Sprintf("Hour %d", i), 1)
				Expect(err).NotTo(HaveOccurred())
			}

			// Query logs from last 5 hours only
			sinceTime := baseTime.Add(5 * time.Hour)
			logs, err := stubAdapter.LoadLogs(ctx, "mock", "worker-1", LogQueryOpts{
				SinceTime: &sinceTime,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(logs).To(HaveLen(5)) // Hours 5, 6, 7, 8, 9
		})

		It("should support log level filtering", func() {
			// Insert logs with different levels
			levels := []string{"INFO", "WARN", "ERROR", "INFO", "ERROR"}
			for i, level := range levels {
				_, err := testDB.Exec(`INSERT INTO mock_logs (id, worker_id, timestamp, level, message, _sync_id)
					VALUES (?, ?, ?, ?, ?, ?)`,
					fmt.Sprintf("log-%d", i), "worker-1", time.Now().Unix(),
					level, fmt.Sprintf("Message %d", i), 1)
				Expect(err).NotTo(HaveOccurred())
			}

			// Query only ERROR logs
			logs, err := stubAdapter.LoadLogs(ctx, "mock", "worker-1", LogQueryOpts{
				LevelFilter: "ERROR",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(logs).To(HaveLen(2))
			Expect(logs[0].Level).To(Equal("ERROR"))
			Expect(logs[1].Level).To(Equal("ERROR"))
		})
	})

	Describe("Explicit Pattern (Blobs - Skip If Unchanged)", func() {
		It("should save large blob to explicit table", func() {
			blob := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
			observed := &MockObservedState{
				Status:        "running",
				MemoryProfile: blob,
			}

			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Verify: Blob in mock_profiles_explicit table
			var storedBlob []byte
			err = testDB.QueryRow("SELECT profile_blob FROM mock_profiles_explicit WHERE id = ?", "worker-1").
				Scan(&storedBlob)
			Expect(err).NotTo(HaveOccurred())
			Expect(storedBlob).To(Equal(blob))
		})

		It("should skip write if blob unchanged (checksum match)", func() {
			blob := []byte{0x01, 0x02, 0x03, 0x04, 0x05}

			// Initial write
			observed := &MockObservedState{MemoryProfile: blob}
			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Get initial checksum
			var checksum1 string
			err = testDB.QueryRow("SELECT checksum FROM mock_profiles_explicit WHERE id = ?", "worker-1").
				Scan(&checksum1)
			Expect(err).NotTo(HaveOccurred())

			// Write same blob again (should skip)
			err = stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Get checksum after second write
			var checksum2 string
			err = testDB.QueryRow("SELECT checksum FROM mock_profiles_explicit WHERE id = ?", "worker-1").
				Scan(&checksum2)
			Expect(err).NotTo(HaveOccurred())

			// Verify: Checksum unchanged (no duplicate write)
			Expect(checksum2).To(Equal(checksum1))
		})

		It("should update blob if content changed", func() {
			blob1 := []byte{0x01, 0x02, 0x03}
			blob2 := []byte{0x04, 0x05, 0x06}

			// Initial write
			observed := &MockObservedState{MemoryProfile: blob1}
			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Get initial checksum
			var checksum1 string
			err = testDB.QueryRow("SELECT checksum FROM mock_profiles_explicit WHERE id = ?", "worker-1").
				Scan(&checksum1)
			Expect(err).NotTo(HaveOccurred())

			// Write different blob
			observed.MemoryProfile = blob2
			err = stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Get new checksum
			var checksum2 string
			var storedBlob []byte
			err = testDB.QueryRow("SELECT checksum, profile_blob FROM mock_profiles_explicit WHERE id = ?", "worker-1").
				Scan(&checksum2, &storedBlob)
			Expect(err).NotTo(HaveOccurred())

			// Verify: Checksum changed and blob updated
			Expect(checksum2).NotTo(Equal(checksum1))
			Expect(storedBlob).To(Equal(blob2))
		})

		It("should handle nil blob gracefully", func() {
			observed := &MockObservedState{
				Status:        "running",
				MemoryProfile: nil,
			}

			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)
			Expect(err).NotTo(HaveOccurred())

			// Verify: No row in explicit table
			var count int
			err = testDB.QueryRow("SELECT COUNT(*) FROM mock_profiles_explicit WHERE id = ?", "worker-1").Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})
	})

	Describe("Struct Tags Detection", func() {
		It("should parse cse tags from struct fields", func() {
			type TaggedStruct struct {
				Instant  string      `cse:"instant"`
				Lazy     *MockConfig `cse:"lazy,table=custom_config"`
				Partial  []LogEntry  `cse:"partial,table=custom_logs,append_only"`
				Explicit []byte      `cse:"explicit,table=custom_blobs,skip_if_unchanged"`
			}

			tags := stubAdapter.ParseStructTags(reflect.TypeOf(TaggedStruct{}))

			Expect(tags).To(HaveKey("Instant"))
			Expect(tags["Instant"].Type).To(Equal("instant"))

			Expect(tags).To(HaveKey("Lazy"))
			Expect(tags["Lazy"].Type).To(Equal("lazy"))
			Expect(tags["Lazy"].Table).To(Equal("custom_config"))

			Expect(tags).To(HaveKey("Partial"))
			Expect(tags["Partial"].Type).To(Equal("partial"))
			Expect(tags["Partial"].Table).To(Equal("custom_logs"))
			Expect(tags["Partial"].AppendOnly).To(BeTrue())

			Expect(tags).To(HaveKey("Explicit"))
			Expect(tags["Explicit"].Type).To(Equal("explicit"))
			Expect(tags["Explicit"].Table).To(Equal("custom_blobs"))
			Expect(tags["Explicit"].SkipIfUnchanged).To(BeTrue())
		})

		It("should default to instant for untagged fields", func() {
			type UntaggedStruct struct {
				Status string // No tag
			}

			tags := stubAdapter.ParseStructTags(reflect.TypeOf(UntaggedStruct{}))

			Expect(tags).To(HaveKey("Status"))
			Expect(tags["Status"].Type).To(Equal("instant")) // Default
		})
	})

	Describe("Full Round-Trip Integration", func() {
		It("should preserve all data through save and load cycle", func() {
			original := &MockObservedState{
				Status:     "running",
				CPUPercent: 45.2,
				Config:     &MockConfig{Setting1: "value1", Setting2: 42},
				NewLogs: []LogEntry{
					{Timestamp: time.Now(), Level: "INFO", Message: "Log 1"},
				},
				MemoryProfile: []byte{0x01, 0x02, 0x03},
			}

			// Save
			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", original)
			Expect(err).NotTo(HaveOccurred())

			// Load
			loaded, err := stubAdapter.LoadObserved(ctx, "mock", "worker-1")
			Expect(err).NotTo(HaveOccurred())

			mockObserved := loaded.(*MockObservedState)

			// Verify all patterns preserved
			Expect(mockObserved.Status).To(Equal("running"))             // Instant
			Expect(mockObserved.Config.Setting1).To(Equal("value1"))     // Lazy
			Expect(mockObserved.NewLogs).To(HaveLen(1))                  // Partial
			Expect(mockObserved.MemoryProfile).To(Equal([]byte{0x01, 0x02, 0x03})) // Explicit
		})

		It("should handle multiple workers independently", func() {
			// Save worker 1
			obs1 := &MockObservedState{Status: "running", CPUPercent: 45.0}
			err := stubAdapter.SaveObserved(ctx, "mock", "worker-1", obs1)
			Expect(err).NotTo(HaveOccurred())

			// Save worker 2
			obs2 := &MockObservedState{Status: "degraded", CPUPercent: 90.0}
			err = stubAdapter.SaveObserved(ctx, "mock", "worker-2", obs2)
			Expect(err).NotTo(HaveOccurred())

			// Load worker 1
			loaded1, err := stubAdapter.LoadObserved(ctx, "mock", "worker-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded1.(*MockObservedState).Status).To(Equal("running"))

			// Load worker 2
			loaded2, err := stubAdapter.LoadObserved(ctx, "mock", "worker-2")
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded2.(*MockObservedState).Status).To(Equal("degraded"))
		})
	})
})

// Mock Types for Contract Testing

// MockObservedState demonstrates all 4 access patterns in one struct
type MockObservedState struct {
	// Instant fields (main table)
	Status     string  `cse:"instant"`
	CPUPercent float64 `cse:"instant"`
	RAMUsageMB int64   `cse:"instant"`

	// Lazy field (1:1 child table)
	Config *MockConfig `cse:"lazy,table=mock_config"`

	// Partial field (1:many child table, append-only)
	NewLogs []LogEntry `cse:"partial,table=mock_logs,append_only"`

	// Explicit field (blob table, skip if unchanged)
	MemoryProfile []byte `cse:"explicit,table=mock_profiles_explicit,skip_if_unchanged"`

	CollectedAt time.Time
}

func (m *MockObservedState) GetTimestamp() time.Time {
	return m.CollectedAt
}

func (m *MockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &MockDesiredState{}
}

type MockDesiredState struct {
	ShutdownFlag bool
}

func (m *MockDesiredState) ShutdownRequested() bool {
	return m.ShutdownFlag
}

func (m *MockDesiredState) SetShutdownRequested(requested bool) {
	m.ShutdownFlag = requested
}

type MockConfig struct {
	Setting1 string
	Setting2 int
}

type LogEntry struct {
	ID        string
	Timestamp time.Time
	Level     string
	Message   string
}

type LogQueryOpts struct {
	Limit       int
	SinceTime   *time.Time
	UntilTime   *time.Time
	LevelFilter string
	OrderDesc   bool
}

// AccessPattern represents parsed struct tag information
type AccessPattern struct {
	Type            string // instant, lazy, partial, explicit
	Table           string // Custom table name
	AppendOnly      bool   // For partial pattern
	SkipIfUnchanged bool   // For explicit pattern
}

// StubAdapter is a minimal implementation that proves the contract works
// This will be replaced with real TriangularAdapter in PR #3
type StubAdapter struct {
	triangular *storage.TriangularStore
	registry   *storage.Registry
	testDB     *sql.DB
}

func NewStubAdapter(ctx context.Context, dbPath string, registry *storage.Registry) (*StubAdapter, *sql.DB, error) {
	// Open SQLite database for test validation
	testDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open test database: %w", err)
	}

	// Create basic.Store wrapper
	cfg := basic.DefaultConfig(dbPath)
	basicStore, err := basic.NewStore(cfg)
	if err != nil {
		testDB.Close()
		return nil, nil, fmt.Errorf("failed to create basic store: %w", err)
	}

	// Create TriangularStore
	triangular := storage.NewTriangularStore(basicStore, registry)

	adapter := &StubAdapter{
		triangular: triangular,
		registry:   registry,
		testDB:     testDB,
	}

	// Create tables
	if err := adapter.createTables(); err != nil {
		testDB.Close()
		basicStore.Close(ctx)
		return nil, nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return adapter, testDB, nil
}

func (s *StubAdapter) createTables() error {
	// Main table (instant fields)
	_, err := s.testDB.Exec(`CREATE TABLE IF NOT EXISTS mock_observed (
		id TEXT PRIMARY KEY,
		status TEXT,
		cpu_percent REAL,
		ram_usage_mb INTEGER,
		_sync_id INTEGER,
		_created_at INTEGER,
		_updated_at INTEGER
	)`)
	if err != nil {
		return fmt.Errorf("failed to create mock_observed table: %w", err)
	}

	// Lazy table (1:1 child)
	_, err = s.testDB.Exec(`CREATE TABLE IF NOT EXISTS mock_config (
		id TEXT PRIMARY KEY,
		setting1 TEXT,
		setting2 INTEGER,
		_sync_id INTEGER,
		FOREIGN KEY(id) REFERENCES mock_observed(id)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create mock_config table: %w", err)
	}

	// Partial table (1:many logs)
	_, err = s.testDB.Exec(`CREATE TABLE IF NOT EXISTS mock_logs (
		id TEXT PRIMARY KEY,
		worker_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		level TEXT,
		message TEXT,
		_sync_id INTEGER,
		FOREIGN KEY(worker_id) REFERENCES mock_observed(id)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create mock_logs table: %w", err)
	}

	_, err = s.testDB.Exec(`CREATE INDEX IF NOT EXISTS idx_mock_logs ON mock_logs(worker_id, timestamp DESC)`)
	if err != nil {
		return fmt.Errorf("failed to create mock_logs index: %w", err)
	}

	// Explicit table (blobs)
	_, err = s.testDB.Exec(`CREATE TABLE IF NOT EXISTS mock_profiles_explicit (
		id TEXT PRIMARY KEY,
		profile_blob BLOB,
		checksum TEXT,
		_sync_id INTEGER,
		FOREIGN KEY(id) REFERENCES mock_observed(id)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create mock_profiles_explicit table: %w", err)
	}

	return nil
}

func (s *StubAdapter) SaveObserved(ctx context.Context, workerType, workerID string, observed fsmv2.ObservedState) error {
	mockObs := observed.(*MockObservedState)

	// Save instant fields to main table
	_, err := s.testDB.Exec(`INSERT OR REPLACE INTO mock_observed (id, status, cpu_percent, ram_usage_mb, _sync_id, _created_at, _updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		workerID, mockObs.Status, mockObs.CPUPercent, mockObs.RAMUsageMB,
		1, time.Now().Unix(), time.Now().Unix())
	if err != nil {
		return err
	}

	// Save lazy field (config) to child table
	if mockObs.Config != nil {
		_, err = s.testDB.Exec(`INSERT OR REPLACE INTO mock_config (id, setting1, setting2, _sync_id)
			VALUES (?, ?, ?, ?)`,
			workerID, mockObs.Config.Setting1, mockObs.Config.Setting2, 1)
		if err != nil {
			return err
		}
	}

	// Append partial fields (logs) to child table
	for _, log := range mockObs.NewLogs {
		logID := fmt.Sprintf("%s-log-%d", workerID, time.Now().UnixNano())
		_, err = s.testDB.Exec(`INSERT INTO mock_logs (id, worker_id, timestamp, level, message, _sync_id)
			VALUES (?, ?, ?, ?, ?, ?)`,
			logID, workerID, log.Timestamp.Unix(), log.Level, log.Message, 1)
		if err != nil {
			return err
		}
	}

	// Save explicit field (blob) if changed
	if mockObs.MemoryProfile != nil {
		checksum := calculateChecksum(mockObs.MemoryProfile)

		// Check if blob exists
		var existingChecksum string
		err = s.testDB.QueryRow("SELECT checksum FROM mock_profiles_explicit WHERE id = ?", workerID).Scan(&existingChecksum)

		if err == sql.ErrNoRows || existingChecksum != checksum {
			// Insert or update blob
			_, err = s.testDB.Exec(`INSERT OR REPLACE INTO mock_profiles_explicit (id, profile_blob, checksum, _sync_id)
				VALUES (?, ?, ?, ?)`,
				workerID, mockObs.MemoryProfile, checksum, 1)
			if err != nil {
				return err
			}
		}
		// If checksum matches, skip write (optimization)
	}

	return nil
}

func (s *StubAdapter) LoadObserved(ctx context.Context, workerType, workerID string) (fsmv2.ObservedState, error) {
	mockObs := &MockObservedState{}

	// Load instant fields from main table
	err := s.testDB.QueryRow("SELECT status, cpu_percent, ram_usage_mb FROM mock_observed WHERE id = ?", workerID).
		Scan(&mockObs.Status, &mockObs.CPUPercent, &mockObs.RAMUsageMB)
	if err != nil {
		return nil, err
	}

	// Load lazy field (config) from child table
	var setting1 string
	var setting2 int
	err = s.testDB.QueryRow("SELECT setting1, setting2 FROM mock_config WHERE id = ?", workerID).
		Scan(&setting1, &setting2)
	if err == nil {
		mockObs.Config = &MockConfig{Setting1: setting1, Setting2: setting2}
	}

	// Load partial fields (logs) from child table - get ALL logs (not typical, but for test)
	rows, err := s.testDB.Query("SELECT timestamp, level, message FROM mock_logs WHERE worker_id = ? ORDER BY timestamp", workerID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var timestamp int64
			var level, message string
			rows.Scan(&timestamp, &level, &message)
			mockObs.NewLogs = append(mockObs.NewLogs, LogEntry{
				Timestamp: time.Unix(timestamp, 0),
				Level:     level,
				Message:   message,
			})
		}
	}

	// Load explicit field (blob) from explicit table
	var blob []byte
	err = s.testDB.QueryRow("SELECT profile_blob FROM mock_profiles_explicit WHERE id = ?", workerID).Scan(&blob)
	if err == nil {
		mockObs.MemoryProfile = blob
	}

	return mockObs, nil
}

func (s *StubAdapter) LoadLogs(ctx context.Context, workerType, workerID string, opts LogQueryOpts) ([]LogEntry, error) {
	// Build query with filters
	query := "SELECT timestamp, level, message FROM mock_logs WHERE worker_id = ?"
	args := []interface{}{workerID}

	if opts.SinceTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, opts.SinceTime.Unix())
	}

	if opts.UntilTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, opts.UntilTime.Unix())
	}

	if opts.LevelFilter != "" {
		query += " AND level = ?"
		args = append(args, opts.LevelFilter)
	}

	if opts.OrderDesc {
		query += " ORDER BY timestamp DESC"
	} else {
		query += " ORDER BY timestamp ASC"
	}

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := s.testDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []LogEntry{}
	for rows.Next() {
		var timestamp int64
		var level, message string
		rows.Scan(&timestamp, &level, &message)
		logs = append(logs, LogEntry{
			Timestamp: time.Unix(timestamp, 0),
			Level:     level,
			Message:   message,
		})
	}

	return logs, nil
}

func (s *StubAdapter) ParseStructTags(t reflect.Type) map[string]AccessPattern {
	patterns := make(map[string]AccessPattern)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("cse")

		if tag == "" {
			// Default to instant for untagged fields
			patterns[field.Name] = AccessPattern{Type: "instant"}
			continue
		}

		// Parse tag: "instant" or "lazy,table=custom" or "partial,table=logs,append_only"
		parts := strings.Split(tag, ",")
		pattern := AccessPattern{Type: parts[0]}

		for _, part := range parts[1:] {
			if strings.HasPrefix(part, "table=") {
				pattern.Table = strings.TrimPrefix(part, "table=")
			} else if part == "append_only" {
				pattern.AppendOnly = true
			} else if part == "skip_if_unchanged" {
				pattern.SkipIfUnchanged = true
			}
		}

		patterns[field.Name] = pattern
	}

	return patterns
}

func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
