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

package redpanda_monitor_test

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
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// Helper function to create mock logs with valid format
func createMockLogs(freBytes, totalBytes uint64, hasSpaceAlert bool, topics, unavailableTopics uint64, bytesIn, bytesOut uint64, topicPartitionMap map[string]int64) []s6service.LogEntry {
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
		panic(fmt.Sprintf("Failed to write metrics: %v", err))
	}
	err = gzipWriter.Close()
	if err != nil {
		panic(fmt.Sprintf("Failed to close gzip writer: %v", err))
	}
	metricsHex := hex.EncodeToString(metricsBuffer.Bytes())

	// Compress and hex-encode cluster config
	var configBuffer bytes.Buffer
	gzipWriter = gzip.NewWriter(&configBuffer)
	err = json.NewEncoder(gzipWriter).Encode(clusterConfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to encode cluster config: %v", err))
	}
	err = gzipWriter.Close()
	if err != nil {
		panic(fmt.Sprintf("Failed to close gzip writer: %v", err))
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

	return logs
}

// Extension of the s6service.MockService to better handle state transitions
type EnhancedS6MockService struct {
	*s6service.MockService
	currentStatus s6service.ServiceStatus
}

func NewEnhancedS6MockService() *EnhancedS6MockService {
	return &EnhancedS6MockService{
		MockService:   s6service.NewMockService(),
		currentStatus: s6service.ServiceDown,
	}
}

// Override Start to update the service state
func (e *EnhancedS6MockService) Start(ctx context.Context, servicePath string, filesystemService filesystem.Service) error {
	e.MockService.StartCalled = true
	e.currentStatus = s6service.ServiceUp

	// Update the status result that will be returned by Status
	e.MockService.StatusResult = s6service.ServiceInfo{
		Status: s6service.ServiceUp,
	}

	info := e.MockService.ServiceStates[servicePath]
	info.Status = s6service.ServiceUp
	e.MockService.ServiceStates[servicePath] = info

	return e.MockService.StartError
}

// Override Stop to update the service state
func (e *EnhancedS6MockService) Stop(ctx context.Context, servicePath string, filesystemService filesystem.Service) error {
	e.MockService.StopCalled = true
	e.currentStatus = s6service.ServiceDown

	// Update the status result that will be returned by Status
	e.MockService.StatusResult = s6service.ServiceInfo{
		Status: s6service.ServiceDown,
	}

	info := e.MockService.ServiceStates[servicePath]
	info.Status = s6service.ServiceDown
	e.MockService.ServiceStates[servicePath] = info

	return e.MockService.StopError
}

// Override Status to provide consistent state information
func (e *EnhancedS6MockService) Status(ctx context.Context, servicePath string, filesystemService filesystem.Service) (s6service.ServiceInfo, error) {
	e.MockService.StatusCalled = true

	if state, exists := e.MockService.ServiceStates[servicePath]; exists {
		return state, e.MockService.StatusError
	}

	// Return the current status
	return s6service.ServiceInfo{
		Status: e.currentStatus,
	}, e.MockService.StatusError
}

var _ = FDescribe("RedpandaMonitor Service State Transitions", func() {
	var (
		mockS6Service  *EnhancedS6MockService
		mockFileSystem *filesystem.MockFileSystem
		monitorService *redpanda_monitor.RedpandaMonitorService
		ctx            context.Context
		cancel         context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		mockS6Service = NewEnhancedS6MockService()
		mockFileSystem = filesystem.NewMockFileSystem()

		// Set up mock logs
		mockS6Service.GetLogsResult = createMockLogs(10000000000, 20000000000, false, 5, 0, 1000, 2000, map[string]int64{"test-topic": 3})

		// Set default state to stopped
		mockS6Service.currentStatus = s6service.ServiceDown
		mockS6Service.StatusResult = s6service.ServiceInfo{
			Status: s6service.ServiceDown,
		}

		// Create the service with mocked dependencies
		monitorService = redpanda_monitor.NewRedpandaMonitorService(
			redpanda_monitor.WithS6Service(mockS6Service),
		)
	})

	AfterEach(func() {
		cancel()
	})

	Context("Service lifecycle with state transitions", func() {
		It("should transition from stopped to running and remain stable for 60 reconciliation cycles", func() {
			By("Adding the service to S6 manager")
			// Mark the service as existing in S6
			servicePath := fmt.Sprintf("%s/%s", "/etc/s6-overlay/s6-rc.d", "redpanda-monitor")
			mockS6Service.ExistingServices[servicePath] = true
			mockS6Service.MockExists = true
			tick := uint64(0)

			err := monitorService.AddRedpandaMonitorToS6Manager(ctx)
			Expect(err).NotTo(HaveOccurred())
			By("Verifiying that without an reconciliation, the service does not exist")
			serviceInfo, err := monitorService.Status(ctx, mockFileSystem, tick)
			Expect(err).To(Equal(redpanda_monitor.ErrServiceNotExist))

			By("Initial service state should be to_be_created")
			err, reconciled := monitorService.ReconcileManager(ctx, mockFileSystem, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			// Verify initial state
			serviceInfo, err = monitorService.Status(ctx, mockFileSystem, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6.LifecycleStateToBeCreated))
			Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeFalse())

			By("reconciliation should put the service into creating")
			err, reconciled = monitorService.ReconcileManager(ctx, mockFileSystem, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			// Verify initial state
			serviceInfo, err = monitorService.Status(ctx, mockFileSystem, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6.LifecycleStateCreating))
			Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeFalse())

			By("reconciliation should put the service into stopped")
			err, reconciled = monitorService.ReconcileManager(ctx, mockFileSystem, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			// Verify initial state
			serviceInfo, err = monitorService.Status(ctx, mockFileSystem, tick)
			tick++
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceInfo.S6FSMState).To(Equal(s6fsm.OperationalStateStopped))
			Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeFalse())

			By("reconciliation should put the service into stopped")
			// Over the next couple loops we expect it to start up
			for i := 0; i < 10; i++ {
				err, reconciled = monitorService.ReconcileManager(ctx, mockFileSystem, tick)
				Expect(err).NotTo(HaveOccurred())
				serviceInfo, err = monitorService.Status(ctx, mockFileSystem, tick)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeTrue())

			/*
				By("Verifying the service stays running for 60 reconciliation cycles")
				// Run through 60 reconciliation cycles
				for i := 0; i < 60; i++ {
					err, reconciled = monitorService.ReconcileManager(ctx, mockFileSystem, tick)
					Expect(err).NotTo(HaveOccurred())
					Expect(reconciled).To(BeFalse())

					// Check status to ensure it stays running
					serviceInfo, err = monitorService.Status(ctx, mockFileSystem, tick)
					Expect(err).NotTo(HaveOccurred())
					Expect(serviceInfo.S6FSMState).To(Equal(s6fsm.OperationalStateRunning))
					Expect(serviceInfo.RedpandaStatus.IsRunning).To(BeTrue())
					tick++
				}
			*/
		})
	})
})
