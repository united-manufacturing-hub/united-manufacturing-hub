package cse_test

import (
	"context"
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

var _ = Describe("TxCache", func() {
	var (
		store    *mockStore
		registry *cse.Registry
		cache    *cse.TxCache
		ctx      context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockStore()
		registry = cse.NewRegistry()
		cache = cse.NewTxCache(store, registry)
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
			cache.BeginTx("tx-123", metadata)

			pending, _ := cache.GetPending()
			Expect(pending[0].TxID).To(Equal("tx-123"))
		})

		It("should set status to pending", func() {
			cache.BeginTx("tx-123", metadata)

			pending, _ := cache.GetPending()
			Expect(pending[0].Status).To(Equal(cse.TxStatusPending))
		})

		It("should preserve metadata", func() {
			cache.BeginTx("tx-123", metadata)

			pending, _ := cache.GetPending()
			Expect(pending[0].Metadata["saga_type"]).To(Equal("deploy_bridge"))
		})
	})

	Describe("RecordOp", func() {
		var op cse.CachedOp

		BeforeEach(func() {
			cache.BeginTx("tx-123", nil)
			op = cse.CachedOp{
				OpType:     "insert",
				Collection: "container_desired",
				ID:         "worker-123",
				Data:       basic.Document{"config": "value"},
				Timestamp:  time.Now(),
			}
		})

		It("should record operation successfully", func() {
			err := cache.RecordOp("tx-123", op)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add operation to transaction", func() {
			cache.RecordOp("tx-123", op)

			pending, _ := cache.GetPending()
			Expect(pending[0].Ops).To(HaveLen(1))
		})

		It("should preserve operation type", func() {
			cache.RecordOp("tx-123", op)

			pending, _ := cache.GetPending()
			Expect(pending[0].Ops[0].OpType).To(Equal("insert"))
		})

		It("should preserve collection name", func() {
			cache.RecordOp("tx-123", op)

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
			cache.BeginTx("tx-123", nil)
		})

		It("should record multiple operations", func() {
			for i := 0; i < 3; i++ {
				op := cse.CachedOp{
					OpType:     "insert",
					Collection: "test",
					ID:         "doc-" + strconv.Itoa(i),
					Data:       basic.Document{"value": i},
					Timestamp:  time.Now(),
				}
				cache.RecordOp("tx-123", op)
			}

			pending, _ := cache.GetPending()
			Expect(pending[0].Ops).To(HaveLen(3))
		})
	})

	Describe("Commit", func() {
		BeforeEach(func() {
			cache.BeginTx("tx-123", nil)
			cache.RecordOp("tx-123", cse.CachedOp{
				OpType:     "insert",
				Collection: "test",
				ID:         "doc-1",
				Data:       basic.Document{"value": 123},
				Timestamp:  time.Now(),
			})
		})

		It("should commit transaction successfully", func() {
			err := cache.Commit("tx-123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove transaction from pending", func() {
			cache.Commit("tx-123")

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
			cache.BeginTx("tx-123", nil)
			cache.RecordOp("tx-123", cse.CachedOp{
				OpType:     "insert",
				Collection: "test",
				ID:         "doc-1",
				Data:       basic.Document{"value": 123},
				Timestamp:  time.Now(),
			})
		})

		It("should rollback transaction successfully", func() {
			err := cache.Rollback("tx-123")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove transaction from pending", func() {
			cache.Rollback("tx-123")

			pending, _ := cache.GetPending()
			Expect(pending).To(BeEmpty())
		})
	})

	Describe("Flush", func() {
		BeforeEach(func() {
			store.CreateCollection(ctx, "_tx_cache", nil)
			cache.BeginTx("tx-123", map[string]interface{}{"saga_type": "deploy"})
			cache.RecordOp("tx-123", cse.CachedOp{
				OpType:     "insert",
				Collection: "test",
				ID:         "doc-1",
				Data:       basic.Document{"value": 123},
				Timestamp:  time.Now(),
			})
		})

		It("should flush to storage successfully", func() {
			err := cache.Flush(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should write transaction to storage", func() {
			cache.Flush(ctx)

			docs, err := store.Find(ctx, "_tx_cache", basic.Query{})
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(1))
		})

		It("should preserve transaction ID in storage", func() {
			cache.Flush(ctx)

			docs, _ := store.Find(ctx, "_tx_cache", basic.Query{})
			Expect(docs[0]["id"]).To(Equal("tx-123"))
		})

		It("should preserve transaction status", func() {
			cache.Flush(ctx)

			docs, _ := store.Find(ctx, "_tx_cache", basic.Query{})
			Expect(docs[0]["status"]).To(Equal(string(cse.TxStatusPending)))
		})

		Context("when transaction is committed", func() {
			BeforeEach(func() {
				cache.Commit("tx-123")
			})

			It("should persist committed status", func() {
				cache.Flush(ctx)

				docs, _ := store.Find(ctx, "_tx_cache", basic.Query{})
				Expect(docs).To(HaveLen(1))
				Expect(docs[0]["status"]).To(Equal(string(cse.TxStatusCommitted)))
			})

			It("should set finished_at timestamp", func() {
				cache.Flush(ctx)

				docs, _ := store.Find(ctx, "_tx_cache", basic.Query{})
				Expect(docs[0]["finished_at"]).NotTo(BeNil())
			})
		})
	})

	Describe("Replay", func() {
		var executedOps []cse.CachedOp

		BeforeEach(func() {
			cache.BeginTx("tx-123", nil)
			cache.RecordOp("tx-123", cse.CachedOp{
				OpType:     "insert",
				Collection: "test",
				ID:         "doc-1",
				Data:       basic.Document{"value": 123},
				Timestamp:  time.Now(),
			})
			cache.RecordOp("tx-123", cse.CachedOp{
				OpType:     "update",
				Collection: "test",
				ID:         "doc-2",
				Data:       basic.Document{"value": 456},
				Timestamp:  time.Now(),
			})

			executedOps = []cse.CachedOp{}
		})

		It("should replay all operations", func() {
			executor := func(op cse.CachedOp) error {
				executedOps = append(executedOps, op)
				return nil
			}

			err := cache.Replay(ctx, "tx-123", executor)
			Expect(err).NotTo(HaveOccurred())
			Expect(executedOps).To(HaveLen(2))
		})

		It("should replay operations in order", func() {
			executor := func(op cse.CachedOp) error {
				executedOps = append(executedOps, op)
				return nil
			}

			cache.Replay(ctx, "tx-123", executor)

			Expect(executedOps[0].ID).To(Equal("doc-1"))
			Expect(executedOps[1].ID).To(Equal("doc-2"))
		})

		Context("when executor returns error", func() {
			It("should fail replay", func() {
				expectedErr := fmt.Errorf("executor failed")
				executor := func(op cse.CachedOp) error {
					return expectedErr
				}

				err := cache.Replay(ctx, "tx-123", executor)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Cleanup", func() {
		BeforeEach(func() {
			store.CreateCollection(ctx, "_tx_cache", nil)
		})

		Context("with old committed transaction", func() {
			BeforeEach(func() {
				cache.BeginTx("tx-old", nil)
				cache.Commit("tx-old")

				cache.BeginTx("tx-new", nil)
				cache.Commit("tx-new")

				cache.BeginTx("tx-pending", nil)

				cache.Flush(ctx)

				oldTime := time.Now().Add(-25 * time.Hour)
				oldDoc, _ := store.Get(ctx, "_tx_cache", "tx-old")
				oldDoc["finished_at"] = oldTime.Format(time.RFC3339)
				store.Update(ctx, "_tx_cache", "tx-old", oldDoc)
			})

			It("should remove old transactions", func() {
				err := cache.Cleanup(ctx, 24*time.Hour)
				Expect(err).NotTo(HaveOccurred())

				docs, _ := store.Find(ctx, "_tx_cache", basic.Query{})
				Expect(docs).To(HaveLen(2))
			})

			It("should not remove recent transactions", func() {
				cache.Cleanup(ctx, 24*time.Hour)

				_, err := store.Get(ctx, "_tx_cache", "tx-new")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not remove pending transactions", func() {
				cache.Cleanup(ctx, 24*time.Hour)

				_, err := store.Get(ctx, "_tx_cache", "tx-pending")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should remove old transaction", func() {
				cache.Cleanup(ctx, 24*time.Hour)

				_, err := store.Get(ctx, "_tx_cache", "tx-old")
				Expect(err).To(MatchError(basic.ErrNotFound))
			})
		})
	})

	Describe("GetPending", func() {
		BeforeEach(func() {
			cache.BeginTx("tx-pending", nil)
			cache.BeginTx("tx-committed", nil)
			cache.Commit("tx-committed")
			cache.BeginTx("tx-failed", nil)
			cache.Rollback("tx-failed")
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

			for i := 0; i < 10; i++ {
				go func(id int) {
					txID := "tx-" + string(rune('0'+id))
					cache.BeginTx(txID, nil)
					cache.RecordOp(txID, cse.CachedOp{
						OpType:     "insert",
						Collection: "test",
						ID:         "doc-" + string(rune('0'+id)),
						Data:       basic.Document{"value": id},
						Timestamp:  time.Now(),
					})
					cache.Commit(txID)
					done <- true
				}(i)
			}

			for i := 0; i < 10; i++ {
				<-done
			}

			pending, _ := cache.GetPending()
			Expect(pending).To(BeEmpty())
		})
	})
})
