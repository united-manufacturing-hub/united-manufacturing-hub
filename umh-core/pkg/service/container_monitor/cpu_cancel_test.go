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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// cpuPanicStateReader panics if the seam reads the observation store. A
// cancelled tick has to be decided before that read, so reaching this method is
// itself the defect, and a panic reports it at the offending call rather than as
// a mismatched return value two frames later.
type cpuPanicStateReader struct{}

func (cpuPanicStateReader) LoadObservedTyped(_ context.Context, _, _ string, _ interface{}) error {
	panic("the CPU seam read the fsmv2 observation store on a cancelled tick")
}

var _ = Describe("the CPU seam on a cancelled tick", func() {
	// Neither spec stages an observation. The seam decides a cancelled tick
	// before it reads anything, so a staged verdict would sit there unread and
	// suggest the outcome depended on it. What the two specs do vary is whether
	// a client was ever published, because the no-client arm is the one the
	// check has to come before: it reports a missing prerequisite, and a
	// cancelled tick has not established that anything is missing.
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
		// Registering the ref is what gives the reader below its reach:
		// GetFresh returns Unregistered without consulting the store when the
		// ref is absent, so without this line the panic could not fire and the
		// spec would assert nothing about read ordering.
		Expect(writer.Upsert(fsmv2cpu.Ref, map[string]any{})).To(Succeed())

		previous := fsmv2client.GetClient()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, cpuPanicStateReader{}))
		DeferCleanup(func() { fsmv2client.SetClient(previous) })

		cpu, err := newService().CollectCPUFromWorker(cancelledContext())

		Expect(err).To(MatchError(context.Canceled))
		Expect(cpu).To(BeNil())
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
