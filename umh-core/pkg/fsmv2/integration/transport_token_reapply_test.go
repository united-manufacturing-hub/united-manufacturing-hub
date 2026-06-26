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

package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"

	// Blank imports register the push and pull child factories so the supervisor
	// can spawn them when the transport parent renders its children.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
)

// tokenReapplyChannelProvider is a minimal transport.ChannelProvider for the
// resident-child token re-apply test. The children are driven only far enough to
// re-derive their desired config; no real push/pull HTTP traffic occurs.
type tokenReapplyChannelProvider struct {
	inbound  chan *types.UMHMessage
	outbound chan *types.UMHMessage
}

func newTokenReapplyChannelProvider() *tokenReapplyChannelProvider {
	return &tokenReapplyChannelProvider{
		inbound:  make(chan *types.UMHMessage, 16),
		outbound: make(chan *types.UMHMessage, 16),
	}
}

func (p *tokenReapplyChannelProvider) GetChannels(_ string) (chan<- *types.UMHMessage, <-chan *types.UMHMessage) {
	return p.inbound, p.outbound
}

func (p *tokenReapplyChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	return cap(p.inbound), len(p.inbound)
}

// childDesiredToken reads the token a resident push/pull child currently holds in
// its persisted desired state. The path mirrors how RenderChildren stamps the auth
// session: WrappedDesiredState marshals the child Config under "config", and the
// push/pull desired Config carries "auth_session".
func childDesiredToken(store storage.TriangularStoreInterface, workerType, workerID string) (string, bool) {
	for _, w := range getWorkersFromStore(store) {
		if w.WorkerType != workerType || w.WorkerID != workerID {
			continue
		}

		cfg, ok := w.Desired["config"].(map[string]any)
		if !ok {
			return "", false
		}

		session, ok := cfg["auth_session"].(map[string]any)
		if !ok {
			return "", false
		}

		token, ok := session["token"].(string)

		return token, ok
	}

	return "", false
}

// childObservedHasValidToken reads the resident push/pull child's observed
// has_valid_token bool. The child sets it from its desired AuthSession via
// IsUsable, so it stays true exactly while the child holds a usable token. The
// second return is false when the child or the field is not yet present.
func childObservedHasValidToken(store storage.TriangularStoreInterface, workerType, workerID string) (bool, bool) {
	for _, w := range getWorkersFromStore(store) {
		if w.WorkerType != workerType || w.WorkerID != workerID {
			continue
		}

		hasToken, ok := w.Observed["has_valid_token"].(bool)

		return hasToken, ok
	}

	return false, false
}

var _ = Describe("Transport resident-child JWT re-apply", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		store        storage.TriangularStoreInterface
		parentSup    *supervisor.Supervisor[fsmv2.Observation[snapshot.TransportStatus], *fsmv2.WrappedDesiredState[snapshot.TransportDesiredState]]
		transportDep *transport.TransportDependencies
	)

	const (
		oldToken = "jwt-token-old"
		newToken = "jwt-token-new"
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		store = setupTestStoreForScenario(deps.NewNopFSMLogger())

		transport.SetChannelProvider(newTokenReapplyChannelProvider())

		parentSup = supervisor.NewSupervisor[fsmv2.Observation[snapshot.TransportStatus], *fsmv2.WrappedDesiredState[snapshot.TransportDesiredState]](supervisor.Config{
			WorkerType:              "transport",
			Store:                   store,
			Logger:                  deps.NewNopFSMLogger(),
			GracefulShutdownTimeout: 10 * time.Second,
		})

		identity := deps.Identity{
			ID:         "transport-001",
			Name:       "Transport",
			WorkerType: "transport",
		}

		worker, err := transport.NewTransportWorker(identity, deps.NewNopFSMLogger(), nil)
		Expect(err).ToNot(HaveOccurred())

		// Publish the parent transport deps the same way the production worker
		// factory does, so the push/pull deps builders can find them.
		register.SetDeps[*transport.TransportDependencies]("transport", worker.GetDependencies())
		transportDep = worker.GetDependencies()

		Expect(parentSup.AddWorker(identity, worker)).To(Succeed())

		// Start the collector goroutines (no owned tick loop) so observed state is
		// refreshed asynchronously; reconciliation is then driven manually via
		// TestTick. Newly spawned children are started the same way and ticked
		// synchronously by the parent each tick.
		parentSup.StartAsChild(ctx)

		// Drive the parent to a running trajectory by giving it a valid spec.
		parentSup.TestUpdateUserSpec(fsmconfig.UserSpec{
			Config: `
relayURL: "http://relay.invalid"
instanceUUID: "test-instance-uuid"
authToken: "test-auth-token"
timeout: "5s"
`,
		})
	})

	AfterEach(func() {
		cancel()
		transport.ClearChannelProvider()
	})

	It("re-applies a rotated JWT to resident push and pull children without despawn", func() {
		// Prime the initial token directly via the production setter so the parent
		// authenticates without a relay round-trip and spawns its children.
		transportDep.SetJWT(oldToken, time.Now().Add(time.Hour))

		// Tick until both children are resident and reflect the initial token.
		Eventually(func() bool {
			_ = parentSup.TestTick(ctx)

			children := parentSup.GetChildren()
			if _, ok := children["push"]; !ok {
				return false
			}

			if _, ok := children["pull"]; !ok {
				return false
			}

			pushTok, pushOK := childDesiredToken(store, "push", "push-001")
			pullTok, pullOK := childDesiredToken(store, "pull", "pull-001")

			return pushOK && pullOK && pushTok == oldToken && pullTok == oldToken
		}, "10s", "100ms").Should(BeTrue(),
			"push and pull children must become resident and carry the initial token")

		// Capture child identities to prove they are NOT recreated by the rotation.
		childrenBefore := parentSup.GetChildren()
		pushBefore := childrenBefore["push"]
		pullBefore := childrenBefore["pull"]
		Expect(pushBefore).ToNot(BeNil())
		Expect(pullBefore).ToNot(BeNil())

		// Baseline: both children observe a usable token before the rotation.
		Eventually(func() bool {
			_ = parentSup.TestTick(ctx)

			pushValid, pushOK := childObservedHasValidToken(store, "push", "push-001")
			pullValid, pullOK := childObservedHasValidToken(store, "pull", "pull-001")

			return pushOK && pullOK && pushValid && pullValid
		}, "10s", "100ms").Should(BeTrue(),
			"push and pull children must observe a valid token before the rotation")

		// Rotate the parent JWT using the same setter production uses on re-auth.
		transportDep.SetJWT(newToken, time.Now().Add(time.Hour))

		// Reconcile on the normal path. No despawn/respawn: the parent stays alive,
		// so the supervisor re-applies the new child config to the resident children.
		// No auth gap: every tick during propagation the child must keep observing a
		// valid token, so it is never left tokenless while the new token lands.
		Eventually(func() bool {
			_ = parentSup.TestTick(ctx)

			pushValid, pushValidOK := childObservedHasValidToken(store, "push", "push-001")
			pullValid, pullValidOK := childObservedHasValidToken(store, "pull", "pull-001")
			Expect(pushValidOK && pushValid).To(BeTrue(),
				"push child must never lose its valid token across the rotation (no auth gap)")
			Expect(pullValidOK && pullValid).To(BeTrue(),
				"pull child must never lose its valid token across the rotation (no auth gap)")

			pushTok, pushOK := childDesiredToken(store, "push", "push-001")
			pullTok, pullOK := childDesiredToken(store, "pull", "pull-001")

			return pushOK && pullOK && pushTok == newToken && pullTok == newToken
		}, "10s", "100ms").Should(BeTrue(),
			"resident push and pull children must pick up the rotated token via config re-apply")

		// The children must be the SAME supervisor instances: re-apply, not respawn.
		childrenAfter := parentSup.GetChildren()
		Expect(childrenAfter["push"]).To(BeIdenticalTo(pushBefore),
			"push child must be re-applied in place, not recreated")
		Expect(childrenAfter["pull"]).To(BeIdenticalTo(pullBefore),
			"pull child must be re-applied in place, not recreated")
		Expect(parentSup.TestIsPendingRemoval("push")).To(BeFalse(),
			"push child must never be marked for removal during a token rotation")
		Expect(parentSup.TestIsPendingRemoval("pull")).To(BeFalse(),
			"pull child must never be marked for removal during a token rotation")
	})
})
