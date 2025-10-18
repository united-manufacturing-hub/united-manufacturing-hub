package cse_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

var _ = Describe("SyncState", func() {
	var (
		store     *mockStore
		registry  *cse.Registry
		syncState *cse.SyncState
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockStore()
		registry = cse.NewRegistry()
		syncState = cse.NewSyncState(store, registry)
	})

	Describe("NewSyncState", func() {
		It("should create non-nil sync state", func() {
			Expect(syncState).NotTo(BeNil())
		})
	})

	Describe("Sync ID tracking", func() {
		Context("for edge tier", func() {
			It("should start at 0", func() {
				Expect(syncState.GetEdgeSyncID()).To(Equal(int64(0)))
			})

			It("should update edge sync ID", func() {
				err := syncState.SetEdgeSyncID(100)
				Expect(err).NotTo(HaveOccurred())
				Expect(syncState.GetEdgeSyncID()).To(Equal(int64(100)))
			})

			It("should update to higher sync ID", func() {
				syncState.SetEdgeSyncID(100)
				syncState.SetEdgeSyncID(200)
				Expect(syncState.GetEdgeSyncID()).To(Equal(int64(200)))
			})
		})

		Context("for relay tier", func() {
			It("should start at 0", func() {
				Expect(syncState.GetRelaySyncID()).To(Equal(int64(0)))
			})

			It("should update relay sync ID", func() {
				err := syncState.SetRelaySyncID(95)
				Expect(err).NotTo(HaveOccurred())
				Expect(syncState.GetRelaySyncID()).To(Equal(int64(95)))
			})

			It("should track relay sync ID independently", func() {
				syncState.SetEdgeSyncID(100)
				syncState.SetRelaySyncID(95)

				Expect(syncState.GetEdgeSyncID()).To(Equal(int64(100)))
				Expect(syncState.GetRelaySyncID()).To(Equal(int64(95)))
			})
		})

		Context("for frontend tier", func() {
			It("should start at 0", func() {
				Expect(syncState.GetFrontendSyncID()).To(Equal(int64(0)))
			})

			It("should update frontend sync ID", func() {
				err := syncState.SetFrontendSyncID(90)
				Expect(err).NotTo(HaveOccurred())
				Expect(syncState.GetFrontendSyncID()).To(Equal(int64(90)))
			})

			It("should track all three tiers independently", func() {
				syncState.SetEdgeSyncID(100)
				syncState.SetRelaySyncID(95)
				syncState.SetFrontendSyncID(90)

				Expect(syncState.GetEdgeSyncID()).To(Equal(int64(100)))
				Expect(syncState.GetRelaySyncID()).To(Equal(int64(95)))
				Expect(syncState.GetFrontendSyncID()).To(Equal(int64(90)))
			})
		})
	})

	Describe("RecordChange", func() {
		It("should track pending changes for edge tier", func() {
			err := syncState.RecordChange(100, cse.TierEdge)
			Expect(err).NotTo(HaveOccurred())

			pending, err := syncState.GetPendingChanges(cse.TierEdge)
			Expect(err).NotTo(HaveOccurred())
			Expect(pending).To(ContainElement(int64(100)))
		})

		It("should track pending changes for relay tier", func() {
			err := syncState.RecordChange(200, cse.TierRelay)
			Expect(err).NotTo(HaveOccurred())

			pending, err := syncState.GetPendingChanges(cse.TierRelay)
			Expect(err).NotTo(HaveOccurred())
			Expect(pending).To(ContainElement(int64(200)))
		})

		It("should track multiple pending changes", func() {
			syncState.RecordChange(100, cse.TierEdge)
			syncState.RecordChange(101, cse.TierEdge)
			syncState.RecordChange(102, cse.TierEdge)

			pending, _ := syncState.GetPendingChanges(cse.TierEdge)
			Expect(pending).To(HaveLen(3))
			Expect(pending).To(ContainElement(int64(100)))
			Expect(pending).To(ContainElement(int64(101)))
			Expect(pending).To(ContainElement(int64(102)))
		})

		It("should keep edge and relay pending changes separate", func() {
			syncState.RecordChange(100, cse.TierEdge)
			syncState.RecordChange(200, cse.TierRelay)

			edgePending, _ := syncState.GetPendingChanges(cse.TierEdge)
			relayPending, _ := syncState.GetPendingChanges(cse.TierRelay)

			Expect(edgePending).To(ContainElement(int64(100)))
			Expect(edgePending).NotTo(ContainElement(int64(200)))
			Expect(relayPending).To(ContainElement(int64(200)))
			Expect(relayPending).NotTo(ContainElement(int64(100)))
		})
	})

	Describe("MarkSynced", func() {
		BeforeEach(func() {
			syncState.RecordChange(100, cse.TierEdge)
			syncState.RecordChange(101, cse.TierEdge)
			syncState.RecordChange(102, cse.TierEdge)
		})

		It("should remove synced changes from pending", func() {
			err := syncState.MarkSynced(cse.TierEdge, 101)
			Expect(err).NotTo(HaveOccurred())

			pending, _ := syncState.GetPendingChanges(cse.TierEdge)
			Expect(pending).NotTo(ContainElement(int64(100)))
			Expect(pending).NotTo(ContainElement(int64(101)))
			Expect(pending).To(ContainElement(int64(102)))
		})

		It("should update relay sync ID when edge syncs", func() {
			syncState.MarkSynced(cse.TierEdge, 101)
			Expect(syncState.GetRelaySyncID()).To(Equal(int64(101)))
		})

		It("should update frontend sync ID when relay syncs", func() {
			syncState.RecordChange(200, cse.TierRelay)
			syncState.RecordChange(201, cse.TierRelay)

			syncState.MarkSynced(cse.TierRelay, 201)
			Expect(syncState.GetFrontendSyncID()).To(Equal(int64(201)))
		})

		It("should handle partial sync", func() {
			syncState.MarkSynced(cse.TierEdge, 100)

			pending, _ := syncState.GetPendingChanges(cse.TierEdge)
			Expect(pending).NotTo(ContainElement(int64(100)))
			Expect(pending).To(ContainElement(int64(101)))
			Expect(pending).To(ContainElement(int64(102)))
		})

		It("should handle complete sync", func() {
			syncState.MarkSynced(cse.TierEdge, 102)

			pending, _ := syncState.GetPendingChanges(cse.TierEdge)
			Expect(pending).To(BeEmpty())
		})
	})

	Describe("GetPendingChanges", func() {
		It("should return empty list initially", func() {
			pending, err := syncState.GetPendingChanges(cse.TierEdge)
			Expect(err).NotTo(HaveOccurred())
			Expect(pending).To(BeEmpty())
		})

		It("should return changes in order", func() {
			syncState.RecordChange(102, cse.TierEdge)
			syncState.RecordChange(100, cse.TierEdge)
			syncState.RecordChange(101, cse.TierEdge)

			pending, _ := syncState.GetPendingChanges(cse.TierEdge)
			Expect(pending).To(HaveLen(3))
		})
	})

	Describe("GetDeltaSince", func() {
		BeforeEach(func() {
			syncState.SetRelaySyncID(100)
		})

		It("should construct query for delta sync", func() {
			query, err := syncState.GetDeltaSince(cse.TierEdge)
			Expect(err).NotTo(HaveOccurred())
			Expect(query).NotTo(BeNil())
		})

		It("should filter by sync ID greater than relay sync ID", func() {
			query, _ := syncState.GetDeltaSince(cse.TierEdge)

			Expect(query.Filters).To(HaveLen(1))
			Expect(query.Filters[0].Field).To(Equal(cse.FieldSyncID))
			Expect(query.Filters[0].Op).To(Equal(basic.Gt))
			Expect(query.Filters[0].Value).To(Equal(int64(100)))
		})

		It("should use edge sync ID for relay tier", func() {
			syncState.SetEdgeSyncID(150)
			query, _ := syncState.GetDeltaSince(cse.TierRelay)

			Expect(query.Filters[0].Value).To(Equal(int64(150)))
		})

		It("should use relay sync ID for frontend tier", func() {
			syncState.SetRelaySyncID(125)
			query, _ := syncState.GetDeltaSince(cse.TierFrontend)

			Expect(query.Filters[0].Value).To(Equal(int64(125)))
		})
	})

	Describe("Flush and Load", func() {
		BeforeEach(func() {
			store.CreateCollection(ctx, "_sync_state", nil)
		})

		It("should persist sync state", func() {
			syncState.SetEdgeSyncID(100)
			syncState.SetRelaySyncID(95)
			syncState.SetFrontendSyncID(90)
			syncState.RecordChange(96, cse.TierEdge)
			syncState.RecordChange(97, cse.TierEdge)

			err := syncState.Flush(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should write document to storage", func() {
			syncState.SetEdgeSyncID(100)
			syncState.Flush(ctx)

			docs, err := store.Find(ctx, "_sync_state", basic.Query{})
			Expect(err).NotTo(HaveOccurred())
			Expect(docs).To(HaveLen(1))
		})

		It("should store sync state as singleton document", func() {
			syncState.SetEdgeSyncID(100)
			syncState.Flush(ctx)

			doc, err := store.Get(ctx, "_sync_state", "sync_state")
			Expect(err).NotTo(HaveOccurred())
			Expect(doc).NotTo(BeNil())
			Expect(doc["id"]).To(Equal("sync_state"))
		})

		It("should restore sync state from storage", func() {
			syncState.SetEdgeSyncID(100)
			syncState.SetRelaySyncID(95)
			syncState.SetFrontendSyncID(90)
			syncState.RecordChange(96, cse.TierEdge)
			syncState.RecordChange(97, cse.TierEdge)
			syncState.Flush(ctx)

			newSyncState := cse.NewSyncState(store, registry)
			err := newSyncState.Load(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(newSyncState.GetEdgeSyncID()).To(Equal(int64(100)))
			Expect(newSyncState.GetRelaySyncID()).To(Equal(int64(95)))
			Expect(newSyncState.GetFrontendSyncID()).To(Equal(int64(90)))

			pending, _ := newSyncState.GetPendingChanges(cse.TierEdge)
			Expect(pending).To(ContainElement(int64(96)))
			Expect(pending).To(ContainElement(int64(97)))
		})

		It("should handle load when no state exists", func() {
			newSyncState := cse.NewSyncState(store, registry)
			err := newSyncState.Load(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(newSyncState.GetEdgeSyncID()).To(Equal(int64(0)))
		})

		It("should update existing state on flush", func() {
			syncState.SetEdgeSyncID(100)
			syncState.Flush(ctx)

			syncState.SetEdgeSyncID(200)
			syncState.Flush(ctx)

			docs, _ := store.Find(ctx, "_sync_state", basic.Query{})
			Expect(docs).To(HaveLen(1))
			Expect(docs[0]["edge_sync_id"]).To(Equal(int64(200)))
		})

		It("should preserve pending changes across flush/load cycle", func() {
			syncState.RecordChange(100, cse.TierEdge)
			syncState.RecordChange(101, cse.TierEdge)
			syncState.RecordChange(200, cse.TierRelay)
			syncState.Flush(ctx)

			newSyncState := cse.NewSyncState(store, registry)
			newSyncState.Load(ctx)

			edgePending, _ := newSyncState.GetPendingChanges(cse.TierEdge)
			relayPending, _ := newSyncState.GetPendingChanges(cse.TierRelay)

			Expect(edgePending).To(HaveLen(2))
			Expect(relayPending).To(HaveLen(1))
		})

		It("should set updated_at timestamp on flush", func() {
			syncState.SetEdgeSyncID(100)
			before := time.Now()
			syncState.Flush(ctx)
			after := time.Now()

			doc, _ := store.Get(ctx, "_sync_state", "sync_state")
			updatedAt, err := time.Parse(time.RFC3339Nano, doc["updated_at"].(string))
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedAt).To(BeTemporally(">=", before))
			Expect(updatedAt).To(BeTemporally("<=", after))
		})
	})

	Describe("ConcurrentAccess", func() {
		It("should handle concurrent sync ID updates safely", func() {
			done := make(chan bool)

			for i := 0; i < 10; i++ {
				go func(id int) {
					syncState.SetEdgeSyncID(int64(id * 10))
					syncState.RecordChange(int64(id*10), cse.TierEdge)
					done <- true
				}(i)
			}

			for i := 0; i < 10; i++ {
				<-done
			}

			pending, _ := syncState.GetPendingChanges(cse.TierEdge)
			Expect(pending).NotTo(BeEmpty())
		})
	})

	Describe("TierConstants", func() {
		It("should define edge tier constant", func() {
			Expect(cse.TierEdge).To(Equal(cse.Tier("edge")))
		})

		It("should define relay tier constant", func() {
			Expect(cse.TierRelay).To(Equal(cse.Tier("relay")))
		})

		It("should define frontend tier constant", func() {
			Expect(cse.TierFrontend).To(Equal(cse.Tier("frontend")))
		})
	})
})
