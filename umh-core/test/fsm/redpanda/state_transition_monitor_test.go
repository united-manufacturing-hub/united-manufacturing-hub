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

package redpanda_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	s6 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// Helper function to create mock logs with valid format
func createMonitorMockLogs(freBytes, totalBytes uint64, hasSpaceAlert bool, topics, unavailableTopics uint64, bytesIn, bytesOut uint64, topicPartitionMap map[string]int64) ([]s6service.LogEntry, error) {
	// Create Prometheus-formatted metrics text
	var promMetrics strings.Builder
	// Add storage metrics
	promMetrics.WriteString("# HELP redpanda_storage_disk_free_bytes Free disk space in bytes\n")
	promMetrics.WriteString("# TYPE redpanda_storage_disk_free_bytes gauge\n")
	promMetrics.WriteString(fmt.Sprintf("redpanda_storage_disk_free_bytes %d\n\n", freBytes))

	promMetrics.WriteString("# HELP redpanda_storage_disk_total_bytes Total disk space in bytes\n")
	promMetrics.WriteString("# TYPE redpanda_storage_disk_total_bytes gauge\n")
	promMetrics.WriteString(fmt.Sprintf("redpanda_storage_disk_total_bytes %d\n\n", totalBytes))

	promMetrics.WriteString("# HELP redpanda_storage_disk_free_space_alert Free disk space alert (0=false, >0=true)\n")
	promMetrics.WriteString("# TYPE redpanda_storage_disk_free_space_alert gauge\n")
	alertValue := 0
	if hasSpaceAlert {
		alertValue = 1
	}
	promMetrics.WriteString(fmt.Sprintf("redpanda_storage_disk_free_space_alert %d\n\n", alertValue))

	// Add cluster metrics
	promMetrics.WriteString("# HELP redpanda_cluster_topics Number of topics in the cluster\n")
	promMetrics.WriteString("# TYPE redpanda_cluster_topics gauge\n")
	promMetrics.WriteString(fmt.Sprintf("redpanda_cluster_topics %d\n\n", topics))

	promMetrics.WriteString("# HELP redpanda_cluster_unavailable_partitions Number of unavailable partitions\n")
	promMetrics.WriteString("# TYPE redpanda_cluster_unavailable_partitions gauge\n")
	promMetrics.WriteString(fmt.Sprintf("redpanda_cluster_unavailable_partitions %d\n\n", unavailableTopics))

	// Add throughput metrics
	promMetrics.WriteString("# HELP redpanda_kafka_request_bytes_total Total bytes in requests by type\n")
	promMetrics.WriteString("# TYPE redpanda_kafka_request_bytes_total counter\n")
	promMetrics.WriteString(fmt.Sprintf("redpanda_kafka_request_bytes_total{redpanda_request=\"produce\"} %d\n", bytesIn))
	promMetrics.WriteString(fmt.Sprintf("redpanda_kafka_request_bytes_total{redpanda_request=\"consume\"} %d\n\n", bytesOut))

	// Add topic partition metrics
	if len(topicPartitionMap) > 0 {
		promMetrics.WriteString("# HELP redpanda_kafka_partitions Number of partitions by topic\n")
		promMetrics.WriteString("# TYPE redpanda_kafka_partitions gauge\n")
		for topic, partitions := range topicPartitionMap {
			promMetrics.WriteString(fmt.Sprintf("redpanda_kafka_partitions{redpanda_topic=\"%s\"} %d\n", topic, partitions))
		}
	}
	// Prom metrics always end with a newline
	promMetrics.WriteString("\n")

	// Create mock cluster config
	clusterConfig := map[string]interface{}{
		"log_retention_ms": 1000000,
		"retention_bytes":  1000000000,
	}

	// Compress and hex-encode metrics data
	var metricsBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&metricsBuffer)
	_, err := gzipWriter.Write([]byte(promMetrics.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to write metrics: %w", err)
	}
	err = gzipWriter.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}
	metricsHex := hex.EncodeToString(metricsBuffer.Bytes())

	// Compress and hex-encode cluster config
	var configBuffer bytes.Buffer
	gzipWriter = gzip.NewWriter(&configBuffer)
	err = json.NewEncoder(gzipWriter).Encode(clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to encode cluster config: %w", err)
	}
	err = gzipWriter.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}
	configHex := hex.EncodeToString(configBuffer.Bytes())

	// Create timestamp
	timestamp := time.Now()

	// Create log entries with the markers
	logs := []s6service.LogEntry{
		{Content: redpanda_monitor.BLOCK_START_MARKER, Timestamp: timestamp},
		{Content: metricsHex, Timestamp: timestamp},
		{Content: redpanda_monitor.METRICS_END_MARKER, Timestamp: timestamp},
		{Content: configHex, Timestamp: timestamp},
		{Content: redpanda_monitor.CLUSTERCONFIG_END_MARKER, Timestamp: timestamp},
		{Content: strconv.FormatInt(timestamp.UnixNano(), 10), Timestamp: timestamp},
		{Content: redpanda_monitor.BLOCK_END_MARKER, Timestamp: timestamp},
	}

	return logs, nil
}

var _ = Describe("RedpandaMonitor Service State Transitions", func() {
	var (
		mockS6Service   *s6service.MockService
		mockSvcRegistry *serviceregistry.Registry
		monitorService  *redpanda_monitor.RedpandaMonitorService
		ctx             context.Context
		cancel          context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		mockS6Service = s6service.NewMockService()
		mockSvcRegistry = serviceregistry.NewMockRegistry()

		// Set up mock logs
		logs, err := createMonitorMockLogs(10000000000, 20000000000, false, 5, 0, 1000, 2000, map[string]int64{"test-topic": 3})
		Expect(err).NotTo(HaveOccurred())
		mockS6Service.GetLogsResult = logs

		// Set default state to stopped
		mockS6Service.StatusResult = s6service.ServiceInfo{
			Status: s6service.ServiceDown,
		}

		// Create a mocked S6 manager with mocked services to prevent using real S6 functionality
		mockedS6Manager := s6fsm.NewS6ManagerWithMockedServices(constants.RedpandaMonitorServiceName)

		// Create the service with mocked dependencies
		monitorService = redpanda_monitor.NewRedpandaMonitorService(
			redpanda_monitor.WithS6Service(mockS6Service),
			redpanda_monitor.WithS6Manager(mockedS6Manager),
		)

		// Set up what happens when AddRedpandaMonitorToS6Manager is called
		// We need to ensure that the instance created by the manager also uses the mock service
		err = monitorService.AddRedpandaMonitorToS6Manager(ctx)
		Expect(err).NotTo(HaveOccurred())
		err, reconciled := monitorService.ReconcileManager(ctx, mockSvcRegistry, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(reconciled).To(BeTrue())
		// Get the instance after reconciliation
		if instance, exists := mockedS6Manager.GetInstance(constants.RedpandaMonitorServiceName); exists {
			// Type assert to S6Instance
			if s6Instance, ok := instance.(*s6fsm.S6Instance); ok {
				// Set our mock service
				s6Instance.SetService(mockS6Service)
			}
		}

	})

	AfterEach(func() {
		cancel()
	})

	Context("Service lifecycle with state transitions", func() {
		It("should transition from stopped to running and remain stable for 60 reconciliation cycles", func() {
			var serviceInfo redpanda_monitor.ServiceInfo
			var err error
			tick := uint64(1) // 1 since we already did one reconciliation in the beforeEach

			By("reconciliation should put the service into creating")
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6.LifecycleStateCreating)
			// Verify initial state
			serviceInfo, err = monitorService.Status(ctx, mockSvcRegistry.GetFileSystem(), tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6.LifecycleStateCreating))
			Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeFalse())
			tick++

			By("reconciliation should put the service into stopped")
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateStopped)
			// Verify state
			serviceInfo, err = monitorService.Status(ctx, mockSvcRegistry.GetFileSystem(), tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6fsm.OperationalStateStopped))
			Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeFalse())
			tick++

			By("Starting the redpanda monitor service")
			// Add implicitly sets the desired state to running, no manualy start is required here

			// Reconcile until the service is running
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateRunning)

			// For the next 1000 iterations, check that the service stays running
			ensureMonitorState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateRunning, 1000)
		})

		It("should transition from running to stopped when requested", func() {
			var serviceInfo redpanda_monitor.ServiceInfo
			var err error
			tick := uint64(1)

			By("Starting up the monitor service")
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6.LifecycleStateCreating)
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateStopped)

			// Add implicitly sets the desired state to running, no manualy start is required here
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateRunning)

			// Verify service is running
			serviceInfo, err = monitorService.Status(ctx, mockSvcRegistry.GetFileSystem(), tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6fsm.OperationalStateRunning))
			Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeTrue())
			tick++

			By("Stopping the redpanda monitor service")
			err = monitorService.StopRedpandaMonitor(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile until the service is stopped
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateStopped)

			// Verify service is stopped
			serviceInfo, err = monitorService.Status(ctx, mockSvcRegistry.GetFileSystem(), tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6fsm.OperationalStateStopped))
			Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeFalse())
		})

		It("should be able to restart after being stopped", func() {
			var serviceInfo redpanda_monitor.ServiceInfo
			var err error
			tick := uint64(1)

			By("Starting up the monitor service")
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6.LifecycleStateCreating)
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateStopped)

			// Add implicitly sets the desired state to running, no manualy start is required here
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateRunning)

			By("Stopping the service")
			err = monitorService.StopRedpandaMonitor(ctx)
			Expect(err).NotTo(HaveOccurred())
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateStopped)

			By("Restarting the service")
			err = monitorService.StartRedpandaMonitor(ctx)
			Expect(err).NotTo(HaveOccurred())
			tick = reconcileMonitorUntilState(ctx, monitorService, mockSvcRegistry, tick, s6fsm.OperationalStateRunning)

			serviceInfo, err = monitorService.Status(ctx, mockSvcRegistry.GetFileSystem(), tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6fsm.OperationalStateRunning))
			Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeTrue())
		})

	})
})

func reconcileMonitorUntilState(ctx context.Context, monitorService *redpanda_monitor.RedpandaMonitorService, services serviceregistry.Provider, tick uint64, expectedState string) uint64 {
	for i := 0; i < 20; i++ {
		err, _ := monitorService.ReconcileManager(ctx, services, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++

		// Check state
		serviceInfo, err := monitorService.Status(ctx, services.GetFileSystem(), tick)
		Expect(err).NotTo(HaveOccurred())
		if serviceInfo.S6FSMState == expectedState {
			return tick
		}
	}

	Fail(fmt.Sprintf("Expected state %s not reached after 10 reconciliations", expectedState))
	return 0
}

func ensureMonitorState(ctx context.Context, monitorService *redpanda_monitor.RedpandaMonitorService, services serviceregistry.Provider, tick uint64, expectedState string, iterations int) {
	for i := 0; i < iterations; i++ {
		err, _ := monitorService.ReconcileManager(ctx, services, tick)
		Expect(err).NotTo(HaveOccurred())
		tick++

		// Check state
		serviceInfo, err := monitorService.Status(ctx, services.GetFileSystem(), tick)
		Expect(err).NotTo(HaveOccurred())
		Expect(serviceInfo.S6FSMState).To(Equal(expectedState))
	}
}
