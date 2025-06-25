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

//go:build integration

package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestIntegration_GraphQLServer(t *testing.T) {
	// Setup test server
	resolver := &Resolver{
		TopicBrowserCache: NewMockTopicBrowserCache(),
	}

	config := &ServerConfig{
		Port:        8899, // Use different port for testing
		Debug:       true,
		CORSOrigins: []string{"*"},
	}

	server, err := NewServer(resolver, config, nil)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server in background
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil {
			t.Logf("Server start error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Ensure cleanup
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Stop(shutdownCtx)
	}()

	// Test GraphQL query
	t.Run("TopicsQuery", func(t *testing.T) {
		query := `{
			topics(limit: 10) {
				topic
				metadata {
					key
					value
				}
				lastEvent {
					... on TimeSeriesEvent {
						producedAt
						scalarType
						stringValue
					}
				}
			}
		}`

		response := executeGraphQLQuery(t, query, 8899)

		// Verify response structure
		if response.Data == nil {
			t.Error("Expected data in response, got nil")
		}

		if len(response.Errors) > 0 {
			t.Errorf("Expected no errors, got: %v", response.Errors)
		}
	})

	t.Run("SingleTopicQuery", func(t *testing.T) {
		query := `{
			topic(topic: "enterprise.site.area.productionLine.workstation.sensor.temperature") {
				topic
				metadata {
					key
					value
				}
			}
		}`

		response := executeGraphQLQuery(t, query, 8899)

		if response.Data == nil {
			t.Error("Expected data in response, got nil")
		}

		if len(response.Errors) > 0 {
			t.Errorf("Expected no errors, got: %v", response.Errors)
		}
	})

	t.Run("CORSHeaders", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}

		req, err := http.NewRequest("OPTIONS", "http://localhost:8899/graphql", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Origin", "http://localhost:3000")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		corsHeader := resp.Header.Get("Access-Control-Allow-Origin")
		if corsHeader == "" {
			t.Error("Expected CORS header, got none")
		}
	})
}

type GraphQLResponse struct {
	Data   interface{}    `json:"data"`
	Errors []GraphQLError `json:"errors"`
}

type GraphQLError struct {
	Message string        `json:"message"`
	Path    []interface{} `json:"path"`
}

func executeGraphQLQuery(t *testing.T, query string, port int) *GraphQLResponse {
	client := &http.Client{Timeout: 5 * time.Second}

	reqBody := map[string]string{
		"query": query,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/graphql", port), bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var response GraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	return &response
}
