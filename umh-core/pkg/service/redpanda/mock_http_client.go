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

package redpanda

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// MockHTTPClient is a mock implementation of HTTPClient for testing
type MockHTTPClient struct {
	// ResponseMap maps endpoint paths to their mock responses
	ResponseMap map[string]MockResponse
}

// MockResponse represents a mock HTTP response
type MockResponse struct {
	StatusCode int
	Body       []byte
	Delay      time.Duration // Simulates response delay for timeout testing
}

// NewMockHTTPClient creates a new mock HTTP client with default responses
func NewMockHTTPClient() *MockHTTPClient {
	client := &MockHTTPClient{
		ResponseMap: make(map[string]MockResponse),
	}

	// Set default metrics response with "healthy" values
	client.SetMetricsResponse(MetricsConfig{
		Infrastructure: InfrastructureMetricsConfig{
			Storage: StorageMetricsConfig{
				FreeBytes:      5000000000,
				TotalBytes:     10000000000,
				FreeSpaceAlert: false,
			},
			Uptime: UptimeMetricsConfig{
				Uptime: 3600, // 1 hour in seconds
			},
		},
		Cluster: ClusterMetricsConfig{
			Topics:            5,
			UnavailableTopics: 0,
		},
		Throughput: ThroughputMetricsConfig{
			BytesIn:  1024,
			BytesOut: 2048,
		},
		Topic: TopicMetricsConfig{
			TopicPartitionMap: map[string]int64{
				"test-topic": 3,
			},
		},
	})

	return client
}

// SetResponse sets a mock response for a specific endpoint
func (m *MockHTTPClient) SetResponse(endpoint string, response MockResponse) {
	m.ResponseMap[endpoint] = response
}

// MetricsConfig represents the configuration for mock metrics
type MetricsConfig struct {
	Infrastructure InfrastructureMetricsConfig
	Cluster        ClusterMetricsConfig
	Throughput     ThroughputMetricsConfig
	Topic          TopicMetricsConfig
}

// InfrastructureMetricsConfig represents infrastructure metrics configuration
type InfrastructureMetricsConfig struct {
	Storage StorageMetricsConfig
	Uptime  UptimeMetricsConfig
}

// StorageMetricsConfig represents storage metrics configuration
type StorageMetricsConfig struct {
	FreeBytes      int64
	TotalBytes     int64
	FreeSpaceAlert bool
}

// UptimeMetricsConfig represents uptime metrics configuration
type UptimeMetricsConfig struct {
	Uptime int64
}

// ClusterMetricsConfig represents cluster metrics configuration
type ClusterMetricsConfig struct {
	Topics            int64
	UnavailableTopics int64
}

// ThroughputMetricsConfig represents throughput metrics configuration
type ThroughputMetricsConfig struct {
	BytesIn  int64
	BytesOut int64
}

// TopicMetricsConfig represents topic metrics configuration
type TopicMetricsConfig struct {
	TopicPartitionMap map[string]int64
}

// SetMetricsResponse sets a custom metrics response
func (m *MockHTTPClient) SetMetricsResponse(config MetricsConfig) {
	var metrics []*dto.MetricFamily

	// Infrastructure metrics - Storage
	metrics = append(metrics,
		createGaugeMetric("redpanda_storage_disk_free_bytes", "Storage free bytes", config.Infrastructure.Storage.FreeBytes),
		createGaugeMetric("redpanda_storage_disk_total_bytes", "Storage total bytes", config.Infrastructure.Storage.TotalBytes),
		createGaugeMetric("redpanda_storage_disk_free_space_alert", "Storage free space alert",
			func() int64 {
				if config.Infrastructure.Storage.FreeSpaceAlert {
					return 1 // Alert active = 1 (Low space)
				}
				return 0 // No alert = 0 (OK)
			}()),
	)

	// Infrastructure metrics - Uptime
	metrics = append(metrics,
		createGaugeMetric("redpanda_uptime_seconds_total", "Uptime in seconds", config.Infrastructure.Uptime.Uptime),
	)

	// Cluster metrics
	metrics = append(metrics,
		createGaugeMetric("redpanda_cluster_topics", "Number of topics", config.Cluster.Topics),
		createGaugeMetric("redpanda_cluster_unavailable_partitions", "Number of unavailable partitions", config.Cluster.UnavailableTopics),
	)

	// Throughput metrics - combine both metrics in a single metric family
	throughputMetrics := []*dto.Metric{
		createMetricWithLabelsForCounter(config.Throughput.BytesIn, map[string]string{"redpanda_request": "produce"}),
		createMetricWithLabelsForCounter(config.Throughput.BytesOut, map[string]string{"redpanda_request": "consume"}),
	}
	metrics = append(metrics,
		createMetricFamily("redpanda_kafka_request_bytes_total", "Kafka request bytes", dto.MetricType_COUNTER, throughputMetrics),
	)

	// Topic metrics - partition count per topic
	if len(config.Topic.TopicPartitionMap) > 0 {
		topicMetrics := make([]*dto.Metric, 0, len(config.Topic.TopicPartitionMap))

		for topic, partitions := range config.Topic.TopicPartitionMap {
			topicMetrics = append(topicMetrics, createMetricWithLabels(partitions, map[string]string{
				"redpanda_topic": topic,
			}))
		}

		metrics = append(metrics,
			createMetricFamily("redpanda_kafka_partitions", "Topic partition count", dto.MetricType_GAUGE, topicMetrics),
		)
	}

	var buf bytes.Buffer
	for _, mf := range metrics {
		expfmt.MetricFamilyToText(&buf, mf)
	}

	// Clear out any existing metrics response before setting the new one
	delete(m.ResponseMap, "/public_metrics")

	m.ResponseMap["/public_metrics"] = MockResponse{
		StatusCode: http.StatusOK,
		Body:       buf.Bytes(),
	}
}

func createMetricWithLabels(value int64, labels map[string]string) *dto.Metric {
	labelPairs := make([]*dto.LabelPair, 0, len(labels))
	for k, v := range labels {
		labelPairs = append(labelPairs, &dto.LabelPair{
			Name:  strPtr(k),
			Value: strPtr(v),
		})
	}

	return &dto.Metric{
		Label: labelPairs,
		Gauge: &dto.Gauge{
			Value: float64Ptr(float64(value)),
		},
	}
}

// Helper function to create a metric with labels specifically for counter metrics
func createMetricWithLabelsForCounter(value int64, labels map[string]string) *dto.Metric {
	labelPairs := make([]*dto.LabelPair, 0, len(labels))
	for k, v := range labels {
		labelPairs = append(labelPairs, &dto.LabelPair{
			Name:  strPtr(k),
			Value: strPtr(v),
		})
	}

	return &dto.Metric{
		Label: labelPairs,
		Counter: &dto.Counter{
			Value: float64Ptr(float64(value)),
		},
	}
}

func createCounterMetricWithLabels(name, help string, value int64, labels map[string]string) *dto.MetricFamily {
	metric := createMetricWithLabels(value, labels)
	metric.Gauge = nil
	metric.Counter = &dto.Counter{
		Value: float64Ptr(float64(value)),
	}
	return createMetricFamily(name, help, dto.MetricType_COUNTER, []*dto.Metric{metric})
}

func createGaugeMetric(name, help string, value int64) *dto.MetricFamily {
	return createMetricFamily(name, help, dto.MetricType_GAUGE, []*dto.Metric{createMetric(value)})
}

func createMetric(value int64) *dto.Metric {
	return &dto.Metric{
		Gauge: &dto.Gauge{
			Value: float64Ptr(float64(value)),
		},
	}
}

func createMetricFamily(name, help string, metricType dto.MetricType, metrics []*dto.Metric) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name:   &name,
		Help:   &help,
		Type:   &metricType,
		Metric: metrics,
	}
}

// Helper functions for creating pointers
func strPtr(s string) *string       { return &s }
func float64Ptr(f float64) *float64 { return &f }

// Do implements the HTTPClient interface
func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if resp, ok := m.ResponseMap[req.URL.Path]; ok {
		if resp.Delay > 0 {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(resp.Delay):
			}
		}
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("connection refused")
		}
		return &http.Response{
			StatusCode: resp.StatusCode,
			Body:       io.NopCloser(bytes.NewReader(resp.Body)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(bytes.NewReader([]byte("not found"))),
	}, nil
}

// SetServiceNotFound configures mock responses to simulate a non-existent service
func (m *MockHTTPClient) SetServiceNotFound(serviceName string) {
	// Set all endpoints to return not found
	notFoundResponse := MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       []byte(fmt.Sprintf("no such service: %s", serviceName)),
	}

	m.ResponseMap["/public_metrics"] = notFoundResponse
}

// GetWithBody performs a mock GET request and returns the response with body
func (m *MockHTTPClient) GetWithBody(ctx context.Context, urlString string) (*http.Response, []byte, error) {
	// Create a request with the given context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlString, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request for %s: %w", urlString, err)
	}

	// Use the Do method to maintain consistent behavior
	resp, err := m.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute request for %s: %w", urlString, err)
	}

	// For mock responses, we can avoid closing the body since it's a NopCloser
	// But we'll read it anyway to simulate real behavior
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("failed to read response body for %s: %w", urlString, err)
	}
	resp.Body.Close()

	// Recreate the response with a fresh body for the caller
	resp.Body = io.NopCloser(bytes.NewReader(body))

	return resp, body, nil
}

// Helper function to convert bool to int64 (0 for false, 1 for true)
func boolToInt64(b bool) int64 {
	if b {
		return 1 // True maps to 1
	}
	return 0 // False maps to 0
}
