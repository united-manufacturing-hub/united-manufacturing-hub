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
	"errors"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("TxCache", func() {
	var (
		store *mockStore
		cache *storage.TxCache
		ctx   context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockStore()
		cache = storage.NewTxCache(store)
	})

	Describe("NewTxCache", func() {
		It("should create non-nil cache", func() {
			Expect(cache).NotTo(BeNil())
		})
	})

	Describe("BeginTx", func() {
		var metadata map[string]interface{}

		BeforeEach(func() {
			metadata = map[string]interface{}{
				"saga_type": "deploy_bridge",
				"worker_id": "worker-123",
			}
		})

		It("should start transaction successfully", func() {
			err := cache.BeginTx("tx-123", metadata)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create pending transaction", func() {
			err := cache.BeginTx("tx-123", metadata)
			Expect(err).NotTo(HaveOccurred())

			pending, err := cache.GetPending()
			Expect(err).NotTo(HaveOccurred())
			Expect(pending).To(HaveLen(1))
		})

		It("should preserve transaction ID", func() {
			err := cache.BeginTx("tx-123", metadata)
			Expect(err).ToNot(HaveOccurred())

			pending, _ := cache.GetPending()
			Expect(pending[0].TxID).To(Equal("tx-123"))
		})

		It("should set status to pending", func() {
			err := cache.BeginTx("tx-123", metadata)
			Expect(err).ToNot(HaveOccurred())

			pending, _ := cache.GetPending()
			Expect(pending[0].Status).To(Equal(storage.TxStatusPending))
		})

		It("should preserve metadata", func() {
			err := cache.BeginTx("tx-123", metadata)
			Expect(err).ToNot(HaveOccurred())

			pending, _ := cache.GetPending()
			Expect(pending[0].Metadata["saga_type"]).To(Equal("deploy_bridge"))
		})
	})

	Describe("RecordOp", func() {
		var op storage.CachedOp

		BeforeEach(func() {
			err := cache.BeginTx("tx-123", nil)
			Expect(err).ToNot(HaveOccurred())
			op = storage.CachedOp{
				OpType:     "insert",
				Collection: "container_desired",
				ID:         "worker-123",
				Data:       persistence.Document{"config": "value"},
				Timestamp:  time.Now(),
			}
		})

		It("should record operation successfully", func() {
			err := cache.RecordOp("tx-123", op)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add operation to transaction", func() {
			err := cache.RecordOp("tx-123", op)
			Expect(err).ToNot(HaveOccurred())

			pending, _ := cache.GetPending()
			Expect(pending[0].Ops).To(HaveLen(1))
		})

		It("should preserve operation type", func() {
			err := cache.RecordOp("tx-123", op)
			Expect(err).ToNot(HaveOccurred())

			pending, _ := cache.GetPending()
			Expect(pending[0].Ops[0].OpType).To(Equal("insert"))
		})

		It("should preserve collection name", func() {
			err := cache.RecordOp("tx-123", op)
			Expect(err).ToNot(HaveOccurred())

			pending, _ := cache.GetPending()
			Expect(pending[0].Ops[0].Collection).To(Equal("container_desired"))
		})

		Context("when transaction does not exist", func() {
			It("should return error", func() {
				err := cache.RecordOp("tx-nonexistent", op)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("RecordMultipleOps", func() {
		BeforeEach(func() {
			err := cache.BeginTx("tx-123", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should record multiple operations", func() {
			for i := range 3 {
				op := storage.CachedOp{
					OpType:     "insert",
					Collection: "test",
					ID:         "doc-" + strconv.Itoa(i),
					Data:       persistence.Document{"value": i},
					Timestamp:  time.Now(),
				}
				err := cache.RecordOp("tx-123", op)
				Expect(err).ToNot(HaveOccurred())
			}

			pending, _ := cache.GetPending()
			Expect(pending[0].Ops).To(HaveLen(3))
		})
	})

	Describe("Commit", func() {
		BeforeEach(func() {
			err := cache.BeginTx("tx-123", nil)
			Expect(err).ToNot(HaveOccurred())
			err = cache.RecordOp("tx-123", storage.CachedOp{
				OpType:     "insert",
				Collection: "test",
				ID:         "doc-1",
				Data:       persistence.Document{"value": 123},
				Timestamp:  time.Now(),
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("should commit transaction successfully", func() {
			err := cache.Commit("tx-123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove transaction from pending", func() {
			err := cache.Commit("tx-123")
			Expect(err).ToNot(HaveOccurred())

			pending, _ := cache.GetPending()
			Expect(pending).To(BeEmpty())
		})

		Context("when transaction does not exist", func() {
			It("should return error", func() {
				err := cache.Commit("tx-nonexistent")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Rollback", func() {
		BeforeEach(func() {
			err := cache.BeginTx("tx-123", nil)
			Expect(err).ToNot(HaveOccurred())
			err = cache.RecordOp("tx-123", storage.CachedOp{
				OpType:     "insert",
				Collection: "test",
				ID:         "doc-1",
				Data:       persistence.Document{"value": 123},
				Timestamp:  time.Now(),
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("should rollback transaction successfully", func() {
			err := cache.Rollback("tx-123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove transaction from pending", func() {
			err := cache.Rollback("tx-123")
			Expect(err).ToNot(HaveOccurred())

			pending, _ := cache.GetPending()
			Expect(pending).To(BeEmpty())
		})
	})

	Describe("Flush", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "_tx_cache", nil)
			Expect(err).ToNot(HaveOccurred())
			err = cache.BeginTx("tx-123", map[string]interface{}{"saga_type": "deploy"})
			Expect(err).ToNot(HaveOccurred())
			err = cache.RecordOp("tx-123", storage.CachedOp{
				OpType:     "insert",
				Collection: "test",
				ID:         "doc-1",
				Data:       persistence.Document{"value": 123},
				Timestamp:  time.Now(),
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("should flush to storage successfully", func() {
			err := cache.Flush(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should write transaction to storage", func() {
			err := cache.Flush(ctx)
			Expect(err).ToNot(HaveOccurred())

			docs, err := store.Find(ctx, "_tx_cache", persistence.Query{})
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(1))
		})

		It("should preserve transaction ID in storage", func() {
			err := cache.Flush(ctx)
			Expect(err).ToNot(HaveOccurred())

			docs, _ := store.Find(ctx, "_tx_cache", persistence.Query{})
			Expect(docs[0]["id"]).To(Equal("tx-123"))
		})

		It("should preserve transaction status", func() {
			err := cache.Flush(ctx)
			Expect(err).ToNot(HaveOccurred())

			docs, _ := store.Find(ctx, "_tx_cache", persistence.Query{})
			Expect(docs[0]["status"]).To(Equal(string(storage.TxStatusPending)))
		})

		Context("when transaction is committed", func() {
			BeforeEach(func() {
				err := cache.Commit("tx-123")
				Expect(err).ToNot(HaveOccurred())
			})

			It("should persist committed status", func() {
				err := cache.Flush(ctx)
				Expect(err).ToNot(HaveOccurred())

				docs, _ := store.Find(ctx, "_tx_cache", persistence.Query{})
				Expect(docs).To(HaveLen(1))
				Expect(docs[0]["status"]).To(Equal(string(storage.TxStatusCommitted)))
			})

			It("should set finished_at timestamp", func() {
				err := cache.Flush(ctx)
				Expect(err).ToNot(HaveOccurred())

				docs, _ := store.Find(ctx, "_tx_cache", persistence.Query{})
				Expect(docs[0]["finished_at"]).NotTo(BeNil())
			})
		})
	})

	Describe("Replay", func() {
		var executedOps []storage.CachedOp

		BeforeEach(func() {
			err := cache.BeginTx("tx-123", nil)
			Expect(err).ToNot(HaveOccurred())
			err = cache.RecordOp("tx-123", storage.CachedOp{
				OpType:     "insert",
				Collection: "test",
				ID:         "doc-1",
				Data:       persistence.Document{"value": 123},
				Timestamp:  time.Now(),
			})
			Expect(err).ToNot(HaveOccurred())
			err = cache.RecordOp("tx-123", storage.CachedOp{
				OpType:     "update",
				Collection: "test",
				ID:         "doc-2",
				Data:       persistence.Document{"value": 456},
				Timestamp:  time.Now(),
			})
			Expect(err).ToNot(HaveOccurred())

			executedOps = []storage.CachedOp{}
		})

		It("should replay all operations", func() {
			executor := func(op storage.CachedOp) error {
				executedOps = append(executedOps, op)

				return nil
			}

			err := cache.Replay(ctx, "tx-123", executor)
			Expect(err).NotTo(HaveOccurred())
			Expect(executedOps).To(HaveLen(2))
		})

		It("should replay operations in order", func() {
			executor := func(op storage.CachedOp) error {
				executedOps = append(executedOps, op)

				return nil
			}

			err := cache.Replay(ctx, "tx-123", executor)
			Expect(err).ToNot(HaveOccurred())

			Expect(executedOps[0].ID).To(Equal("doc-1"))
			Expect(executedOps[1].ID).To(Equal("doc-2"))
		})

		Context("when executor returns error", func() {
			It("should fail replay", func() {
				expectedErr := errors.New("executor failed")
				executor := func(op storage.CachedOp) error {
					return expectedErr
				}

				err := cache.Replay(ctx, "tx-123", executor)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Cleanup", func() {
		BeforeEach(func() {
			err := store.CreateCollection(ctx, "_tx_cache", nil)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("with old committed transaction", func() {
			BeforeEach(func() {
				err := cache.BeginTx("tx-old", nil)
				Expect(err).ToNot(HaveOccurred())
				err = cache.Commit("tx-old")
				Expect(err).ToNot(HaveOccurred())

				err = cache.BeginTx("tx-new", nil)
				Expect(err).ToNot(HaveOccurred())
				err = cache.Commit("tx-new")
				Expect(err).ToNot(HaveOccurred())

				err = cache.BeginTx("tx-pending", nil)
				Expect(err).ToNot(HaveOccurred())

				err = cache.Flush(ctx)
				Expect(err).ToNot(HaveOccurred())

				oldTime := time.Now().Add(-25 * time.Hour)
				oldDoc, _ := store.Get(ctx, "_tx_cache", "tx-old")
				oldDoc["finished_at"] = oldTime.Format(time.RFC3339)
				err = store.Update(ctx, "_tx_cache", "tx-old", oldDoc)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should remove old transactions", func() {
				err := cache.Cleanup(ctx, 24*time.Hour)
				Expect(err).NotTo(HaveOccurred())

				docs, _ := store.Find(ctx, "_tx_cache", persistence.Query{})
				Expect(docs).To(HaveLen(2))
			})

			It("should not remove recent transactions", func() {
				err := cache.Cleanup(ctx, 24*time.Hour)
				Expect(err).ToNot(HaveOccurred())

				_, err = store.Get(ctx, "_tx_cache", "tx-new")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not remove pending transactions", func() {
				err := cache.Cleanup(ctx, 24*time.Hour)
				Expect(err).ToNot(HaveOccurred())

				_, err = store.Get(ctx, "_tx_cache", "tx-pending")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should remove old transaction", func() {
				err := cache.Cleanup(ctx, 24*time.Hour)
				Expect(err).ToNot(HaveOccurred())

				_, err = store.Get(ctx, "_tx_cache", "tx-old")
				Expect(err).To(MatchError(persistence.ErrNotFound))
			})
		})
	})

	Describe("GetPending", func() {
		BeforeEach(func() {
			err := cache.BeginTx("tx-pending", nil)
			Expect(err).ToNot(HaveOccurred())
			err = cache.BeginTx("tx-committed", nil)
			Expect(err).ToNot(HaveOccurred())
			err = cache.Commit("tx-committed")
			Expect(err).ToNot(HaveOccurred())
			err = cache.BeginTx("tx-failed", nil)
			Expect(err).ToNot(HaveOccurred())
			err = cache.Rollback("tx-failed")
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return only pending transactions", func() {
			pending, err := cache.GetPending()
			Expect(err).NotTo(HaveOccurred())
			Expect(pending).To(HaveLen(1))
		})

		It("should return correct pending transaction", func() {
			pending, _ := cache.GetPending()
			Expect(pending[0].TxID).To(Equal("tx-pending"))
		})
	})

	Describe("ConcurrentAccess", func() {
		It("should handle concurrent transactions safely", func() {
			done := make(chan bool)

			for i := range 10 {
				go func(id int) {
					txID := "tx-" + string(rune('0'+id))
					err := cache.BeginTx(txID, nil)
					Expect(err).ToNot(HaveOccurred())
					err = cache.RecordOp(txID, storage.CachedOp{
						OpType:     "insert",
						Collection: "test",
						ID:         "doc-" + string(rune('0'+id)),
						Data:       persistence.Document{"value": id},
						Timestamp:  time.Now(),
					})
					Expect(err).ToNot(HaveOccurred())
					err = cache.Commit(txID)
					Expect(err).ToNot(HaveOccurred())
					done <- true
				}(i)
			}

			for range 10 {
				<-done
			}

			pending, _ := cache.GetPending()
			Expect(pending).To(BeEmpty())
		})
	})
})
