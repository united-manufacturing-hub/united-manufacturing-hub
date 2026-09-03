package container_monitor_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("the CPU seam on a cancelled tick", func() {
	It("should abort rather than publish a degraded verdict nothing measured", func() {
		Expect(os.Setenv("USE_FSMV2_CPU", "true")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("USE_FSMV2_CPU") })

		writer := dynamicchildren.NewWriter()
		Expect(writer.Upsert(fsmv2cpu.Ref, map[string]any{})).To(Succeed())
		stub := &cpuStubStateReader{obs: &fsmv2.Observation[simple.Status[fsmv2cpu.CPUStatus]]{
			CollectedAt: time.Now(),
			Status: simple.Status[fsmv2cpu.CPUStatus]{
				Result: fsmv2cpu.CPUStatus{
					Verdict: cpuhealth.Verdict{State: cpuhealth.StateHealthy},
					Details: workerHealthyDetails,
					Message: workerHealthyMessage,
				},
			},
		}}
		previous := fsmv2client.GetClient()
		fsmv2client.SetClient(fsmv2client.NewFSMv2Client(writer, stub))
		DeferCleanup(func() { fsmv2client.SetClient(previous) })

		dir, err := os.MkdirTemp("", "cpu-cancel")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = os.RemoveAll(dir) })

		service := container_monitor.NewContainerMonitorServiceWithPath(filesystem.NewMockFileSystem(), dir)

		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		// A store that honours ctx returns the cancellation as a read error.
		stub.err = context.Canceled

		cpu, err := service.CollectCPUFromWorker(cancelledCtx)
		Expect(err).To(MatchError(context.Canceled))
		Expect(cpu).To(BeNil())
	})
})
