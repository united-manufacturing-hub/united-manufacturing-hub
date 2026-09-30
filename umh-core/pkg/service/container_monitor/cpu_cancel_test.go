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

package container_monitor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// cpuPanicStateReader panics if the seam reads the observation store. A
// cancelled tick has to be decided before that read, so reaching this method is
// itself the defect.
type cpuPanicStateReader struct{}

func (cpuPanicStateReader) LoadObservedTyped(_ context.Context, _, _ string, _ interface{}) error {
	panic("the CPU seam read the fsmv2 observation store on a cancelled tick")
}

// Every shape a cancelled tick can arrive in. Add one below and it is covered.
// The shape with a store that serves its observation successfully is the only
// one production can produce, and it is the one a check conjoined with the read
// error would miss.
var _ = Describe("the CPU seam on a cancelled tick", func() {
	newService := func() *container_monitor.ContainerMonitorService {
		// The path is never read: the seam returns before it touches the
		// filesystem.
		return container_monitor.NewContainerMonitorServiceWithPath(
			filesystem.NewMockFileSystem(), "/unused-by-a-cancelled-tick")
	}

	cancelledContext := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		return ctx
	}

	It("should abort without reading the store, even with a client published", func() {
		writer := dynamicchildren.NewWriter()
		// Registering the ref is what makes this spec able to fail. Remove the
		// ctx check in readWorkerCPUHealth and the seam reaches GetFresh, which
		// consults the store only for a registered ref -- so without this line
		// the panic could not fire and nothing here would notice.
		Expect(writer.Upsert(fsmv2cpu.Ref, map[string]any{})).To(Succeed())

		previous := fsmv2client.GetClient()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, cpuPanicStateReader{}))
		DeferCleanup(func() { fsmv2client.SetClient(previous) })

		cpu, err := newService().CollectCPUFromWorker(cancelledContext())

		Expect(err).To(MatchError(context.Canceled))
		Expect(cpu).To(BeNil())
	})

	It("should abort when the store serves its observation despite the cancellation", func() {
		// The shape production actually produces. No store here consults ctx on
		// a read, so the read succeeds and returns a healthy verdict while the
		// context is already cancelled, and only ctx.Err() catches it.
		writer := dynamicchildren.NewWriter()
		Expect(writer.Upsert(fsmv2cpu.Ref, map[string]any{})).To(Succeed())

		serving := &cpuStubStateReader{obs: &fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
			CollectedAt: time.Now(),
			Status:      healthyWorkerStatus(),
		}}

		previous := fsmv2client.GetClient()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, serving))
		DeferCleanup(func() { fsmv2client.SetClient(previous) })

		cpu, err := newService().CollectCPUFromWorker(cancelledContext())

		Expect(err).To(MatchError(context.Canceled))
		Expect(cpu).To(BeNil(), "a cancelled tick reports nothing, healthy or otherwise")
	})

	It("should abort with no client published, rather than blame a missing prerequisite", func() {
		previous := fsmv2client.GetClient()
		fsmv2client.SetClient(nil)
		DeferCleanup(func() { fsmv2client.SetClient(previous) })

		cpu, err := newService().CollectCPUFromWorker(cancelledContext())

		Expect(err).To(MatchError(context.Canceled))
		Expect(cpu).To(BeNil())
	})
})
