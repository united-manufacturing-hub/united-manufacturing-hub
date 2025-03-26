package generator

import (
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
)

const (
	defaultCacheExpiration = 1 * time.Hour
	defaultCacheCullPeriod = 10 * time.Minute
)

type StatusCollectorType struct {
	latestData *LatestData
	dog        watchdog.Iface
}

type LatestData struct {
	mu sync.RWMutex // A mutex to synchronize access to the fields

	UnsTable   models.UnsTable
	EventTable models.EventTable
}

func NewStatusCollector(
	dog watchdog.Iface,
) *StatusCollectorType {

	latestData := &LatestData{}

	collector := &StatusCollectorType{
		latestData: latestData,
		dog:        dog,
	}

	return collector
}

func (s *StatusCollectorType) GenerateStatusMessage() *models.StatusMessage {
	s.latestData.mu.RLock()
	defer s.latestData.mu.RUnlock()

	return &models.StatusMessage{
		Core: models.Core{
			Agent: models.Agent{
				Health: models.Health{
					Message: "Agent is healthy",
					State:   "running",
				},
			},
			Container: models.Container{
				Architecture: "x86_64",
				CPU: models.CPU{
					Health: models.Health{
						Message: "CPU usage normal",
						State:   "healthy",
					},
					Limit: 4.0,
					Usage: 35.5,
				},
				Disk: models.Disk{
					Health: models.Health{
						Message: "Disk usage normal",
						State:   "healthy",
					},
					Limit: 100 * 1024 * 1024 * 1024, // 100GB
					Usage: 45 * 1024 * 1024 * 1024,  // 45GB
				},
				Memory: models.Memory{
					Health: models.Health{
						Message: "Memory usage normal",
						State:   "healthy",
					},
					Limit: 16 * 1024 * 1024 * 1024, // 16GB
					Usage: 8 * 1024 * 1024 * 1024,  // 8GB
				},
				Hwid: "mock-hwid-123",
			},
			Dfcs: []models.Dfc{
				{
					UUID:               "mock-dfc-123",
					DfcType:            models.ProtocolConverter,
					DeploySuccess:      true,
					CurrentVersionUUID: stringPtr("v1.0.0"),
					Name:               stringPtr("Mock DFC"),
					Health: &models.Health{
						Message: "DFC is healthy",
						State:   "running",
					},
					Metrics: &models.DFCMetrics{
						FailedMessages:      0,
						ThroughputMsgPerSEC: 100.5,
						Unprocessable:       0,
						Unprocessable24H:    0,
					},
					Connections: []models.Connection{
						{
							Health: models.Health{
								Message: "Connection stable",
								State:   "connected",
							},
							Latency: 5.5,
							Name:    "Mock Connection",
							URI:     "opc.tcp://mock-server:4840",
							UUID:    "mock-conn-123",
						},
					},
				},
			},
			EventsTable: make(map[string]models.EventsTable),
			Latency: models.Latency{
				Avg: 10.5,
				Max: 50.0,
				Min: 1.0,
				P95: 25.0,
				P99: 45.0,
			},
			Redpanda: models.Redpanda{
				Health: models.Health{
					Message: "Redpanda is healthy",
					State:   "running",
				},
				ThroughputIncomingMsgPerSEC: 1000.5,
				ThroughputOutgoingMsgPerSEC: 950.3,
			},
			ReleaseChannel:    "stable",
			SupportedFeatures: []string{"protocol-converter", "data-bridge", "custom"},
			UnsTable:          make(map[string]models.UnsTable),
			Version:           "v1.0.0",
		},
		General: models.General{
			Location: map[string]string{
				"building": "Mock Building",
				"floor":    "1st Floor",
				"area":     "Production Line 1",
			},
		},
		Plugins: make(map[string]interface{}),
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
