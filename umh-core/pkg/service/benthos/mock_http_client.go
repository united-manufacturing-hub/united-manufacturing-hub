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

package benthos

import (
	"bytes"
	"context"
	"encoding/json"
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

	// Set default responses
	client.SetResponse("/ping", MockResponse{
		StatusCode: http.StatusOK,
	})

	client.SetResponse("/version", MockResponse{
		StatusCode: http.StatusOK,
		Body:       []byte(`{"version":"mock-version","built":"mock-built"}`),
	})

	// Set default ready response
	client.SetReadyStatus(http.StatusOK, true, true, "")

	// Set default metrics response
	client.SetMetricsResponse(MetricsConfig{
		Input: MetricsConfigInput{
			ConnectionUp: 1,
			Received:     6,
			LatencyNS: LatencyConfig{
				P50:   196417,
				P90:   895875,
				P99:   895875,
				Sum:   2023542,
				Count: 6,
			},
		},
		Output: MetricsConfigOutput{
			BatchSent:    6,
			ConnectionUp: 1,
			Sent:         6,
			LatencyNS: LatencyConfig{
				P50:   59000,
				P90:   127916,
				P99:   127916,
				Sum:   505124,
				Count: 6,
			},
		},
	})

	return client
}

// SetResponse sets a mock response for a specific endpoint
func (m *MockHTTPClient) SetResponse(endpoint string, response MockResponse) {
	m.ResponseMap[endpoint] = response
}

// SetReadyStatus sets a custom ready response
func (m *MockHTTPClient) SetReadyStatus(statusCode int, inputConnected bool, outputConnected bool, errorMsg string) {
	resp := readyResponse{
		Error: errorMsg,
		Statuses: []connStatus{
			{
				Label:     "",
				Path:      "input",
				Connected: inputConnected,
				Error:     errorMsg,
			},
			{
				Label:     "",
				Path:      "output",
				Connected: outputConnected,
				Error:     errorMsg,
			},
		},
	}

	body, err := json.Marshal(resp)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal ready response: %v", err))
	}

	m.ResponseMap["/ready"] = MockResponse{
		StatusCode: statusCode,
		Body:       body,
	}
}

// MetricsConfig represents the configuration for mock metrics
type MetricsConfig struct {
	Input      MetricsConfigInput
	Output     MetricsConfigOutput
	Processors []ProcessorMetricsConfig
}

// MetricsConfigInput represents input metrics configuration
type MetricsConfigInput struct {
	ConnectionFailed int64
	ConnectionLost   int64
	ConnectionUp     int64
	Received         int64
	LatencyNS        LatencyConfig
}

// MetricsConfigOutput represents output metrics configuration
type MetricsConfigOutput struct {
	BatchSent        int64
	ConnectionFailed int64
	ConnectionLost   int64
	ConnectionUp     int64
	Error            int64
	Sent             int64
	LatencyNS        LatencyConfig
}

// ProcessorMetricsConfig represents processor metrics configuration
type ProcessorMetricsConfig struct {
	Path          string // e.g. "root.pipeline.processors.0"
	Label         string // e.g. "0"
	Received      int64
	BatchReceived int64
	Sent          int64
	BatchSent     int64
	Error         int64
	LatencyNS     LatencyConfig
}

// LatencyConfig represents latency metrics configuration
type LatencyConfig struct {
	P50   float64
	P90   float64
	P99   float64
	Sum   float64
	Count int64
}

// SetMetricsResponse sets a custom metrics response
func (m *MockHTTPClient) SetMetricsResponse(config MetricsConfig) {
	var metrics []*dto.MetricFamily

	// Input metrics
	metrics = append(metrics,
		createCounterMetric("input_connection_failed", "Benthos Counter metric", config.Input.ConnectionFailed, "root.input", ""),
		createCounterMetric("input_connection_lost", "Benthos Counter metric", config.Input.ConnectionLost, "root.input", ""),
		createCounterMetric("input_connection_up", "Benthos Counter metric", config.Input.ConnectionUp, "root.input", ""),
		createCounterMetric("input_received", "Benthos Counter metric", config.Input.Received, "root.input", ""),
		createSummaryMetric("input_latency_ns", "Benthos Timing metric", config.Input.LatencyNS, "root.input", ""),
	)

	// Output metrics
	metrics = append(metrics,
		createCounterMetric("output_batch_sent", "Benthos Counter metric", config.Output.BatchSent, "root.output", ""),
		createCounterMetric("output_connection_failed", "Benthos Counter metric", config.Output.ConnectionFailed, "root.output", ""),
		createCounterMetric("output_connection_lost", "Benthos Counter metric", config.Output.ConnectionLost, "root.output", ""),
		createCounterMetric("output_connection_up", "Benthos Counter metric", config.Output.ConnectionUp, "root.output", ""),
		createCounterMetric("output_error", "Benthos Counter metric", config.Output.Error, "root.output", ""),
		createCounterMetric("output_sent", "Benthos Counter metric", config.Output.Sent, "root.output", ""),
		createSummaryMetric("output_latency_ns", "Benthos Timing metric", config.Output.LatencyNS, "root.output", ""),
	)

	// Processor metrics - group by metric name
	if len(config.Processors) > 0 {
		// Create arrays for each type of processor metric
		receivedMetrics := make([]*dto.Metric, 0, len(config.Processors))
		batchReceivedMetrics := make([]*dto.Metric, 0, len(config.Processors))
		sentMetrics := make([]*dto.Metric, 0, len(config.Processors))
		batchSentMetrics := make([]*dto.Metric, 0, len(config.Processors))
		errorMetrics := make([]*dto.Metric, 0, len(config.Processors))
		latencyMetrics := make([]*dto.Metric, 0, len(config.Processors))

		for _, proc := range config.Processors {
			receivedMetrics = append(receivedMetrics, createMetric(proc.Received, proc.Path, proc.Label))
			batchReceivedMetrics = append(batchReceivedMetrics, createMetric(proc.BatchReceived, proc.Path, proc.Label))
			sentMetrics = append(sentMetrics, createMetric(proc.Sent, proc.Path, proc.Label))
			batchSentMetrics = append(batchSentMetrics, createMetric(proc.BatchSent, proc.Path, proc.Label))
			errorMetrics = append(errorMetrics, createMetric(proc.Error, proc.Path, proc.Label))
			latencyMetrics = append(latencyMetrics, createSummaryMetricValue(proc.LatencyNS, proc.Path, proc.Label))
		}

		// Add all processor metrics as single families
		metrics = append(metrics,
			createMetricFamily("processor_received", "Benthos Counter metric", dto.MetricType_COUNTER, receivedMetrics),
			createMetricFamily("processor_batch_received", "Benthos Counter metric", dto.MetricType_COUNTER, batchReceivedMetrics),
			createMetricFamily("processor_sent", "Benthos Counter metric", dto.MetricType_COUNTER, sentMetrics),
			createMetricFamily("processor_batch_sent", "Benthos Counter metric", dto.MetricType_COUNTER, batchSentMetrics),
			createMetricFamily("processor_error", "Benthos Counter metric", dto.MetricType_COUNTER, errorMetrics),
			createMetricFamily("processor_latency_ns", "Benthos Timing metric", dto.MetricType_SUMMARY, latencyMetrics),
		)
	}

	var buf bytes.Buffer
	for _, mf := range metrics {
		_, err := expfmt.MetricFamilyToText(&buf, mf)
		if err != nil {
			panic(fmt.Sprintf("Failed to marshal metrics: %v", err))
		}
	}

	m.ResponseMap["/metrics"] = MockResponse{
		StatusCode: http.StatusOK,
		Body:       buf.Bytes(),
	}
}

func createMetric(value int64, path, label string) *dto.Metric {
	return &dto.Metric{
		Label: []*dto.LabelPair{
			{
				Name:  strPtr("label"),
				Value: strPtr(label),
			},
			{
				Name:  strPtr("path"),
				Value: strPtr(path),
			},
		},
		Counter: &dto.Counter{
			Value: float64Ptr(float64(value)),
		},
	}
}

func createSummaryMetricValue(latency LatencyConfig, path, label string) *dto.Metric {
	return &dto.Metric{
		Label: []*dto.LabelPair{
			{
				Name:  strPtr("label"),
				Value: strPtr(label),
			},
			{
				Name:  strPtr("path"),
				Value: strPtr(path),
			},
		},
		Summary: &dto.Summary{
			SampleCount: uint64Ptr(uint64(latency.Count)),
			SampleSum:   float64Ptr(latency.Sum),
			Quantile: []*dto.Quantile{
				{
					Quantile: float64Ptr(0.5),
					Value:    float64Ptr(latency.P50),
				},
				{
					Quantile: float64Ptr(0.9),
					Value:    float64Ptr(latency.P90),
				},
				{
					Quantile: float64Ptr(0.99),
					Value:    float64Ptr(latency.P99),
				},
			},
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

func createCounterMetric(name, help string, value int64, path, label string) *dto.MetricFamily {
	return createMetricFamily(name, help, dto.MetricType_COUNTER, []*dto.Metric{createMetric(value, path, label)})
}

func createSummaryMetric(name, help string, latency LatencyConfig, path, label string) *dto.MetricFamily {
	return createMetricFamily(name, help, dto.MetricType_SUMMARY, []*dto.Metric{createSummaryMetricValue(latency, path, label)})
}

// Helper functions for creating pointers
func strPtr(s string) *string       { return &s }
func float64Ptr(f float64) *float64 { return &f }
func uint64Ptr(i uint64) *uint64    { return &i }

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

// SetLivenessProbe sets up a mock response for the liveness probe endpoint
func (m *MockHTTPClient) SetLivenessProbe(statusCode int, alive bool) {
	body := []byte("OK")
	if !alive {
		body = []byte("service unavailable")
	}
	m.ResponseMap["/ping"] = MockResponse{
		StatusCode: statusCode,
		Body:       body,
	}
}

// SetServiceNotFound configures mock responses to simulate a non-existent service
func (m *MockHTTPClient) SetServiceNotFound(serviceName string) {
	// Set all endpoints to return not found
	notFoundResponse := MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       []byte(fmt.Sprintf("no such service: %s", serviceName)),
	}

	m.ResponseMap["/ping"] = notFoundResponse
	m.ResponseMap["/ready"] = notFoundResponse
	m.ResponseMap["/metrics"] = notFoundResponse
	m.ResponseMap["/version"] = notFoundResponse
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
	err = resp.Body.Close()
	if err != nil {
		return resp, nil, fmt.Errorf("failed to close response body for %s: %w", urlString, err)
	}

	// Recreate the response with a fresh body for the caller
	resp.Body = io.NopCloser(bytes.NewReader(body))

	return resp, body, nil
}
