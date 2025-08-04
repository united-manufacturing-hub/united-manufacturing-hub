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

//go:build manual

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// This is a standalone program to test the mock server independently
// Build with: go build -tags manual test_mock_server.go mock_server.go
// Run with: ./test_mock_server
func main() {
	fmt.Println("Starting mock API server for testing...")

	// Create and start the mock server
	server := NewMockAPIServer()
	err := server.Start()
	if err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		os.Exit(1)
	}
	defer server.Stop()

	fmt.Printf("Mock API server running on %s\n", server.GetURL())
	fmt.Println("Endpoints available:")
	fmt.Printf("  POST %s/v2/instance/login\n", server.GetURL())
	fmt.Printf("  GET  %s/v2/instance/pull\n", server.GetURL())
	fmt.Printf("  POST %s/v2/instance/push\n", server.GetURL())

	// Add some test messages to the pull queue
	testMessages := []models.UMHMessage{
		{
			Metadata: &models.MessageMetadata{
				TraceID: uuid.New(),
			},
			Email:        "test1@example.com",
			Content:      "First test message",
			InstanceUUID: uuid.New(),
		},
		{
			Metadata: &models.MessageMetadata{
				TraceID: uuid.New(),
			},
			Email:        "test2@example.com",
			Content:      "Second test message",
			InstanceUUID: uuid.New(),
		},
	}

	for _, msg := range testMessages {
		server.AddMessageToPullQueue(msg)
	}

	fmt.Printf("Added %d test messages to pull queue\n", len(testMessages))
	fmt.Println("\nTo test manually:")
	fmt.Println("1. First login:")
	fmt.Printf("   curl -X POST %s/v2/instance/login -H 'Authorization: Bearer test-token' -v\n", server.GetURL())
	fmt.Println("2. Then pull messages (use the JWT from login response):")
	fmt.Printf("   curl -X GET %s/v2/instance/pull -H 'Cookie: token=YOUR_JWT_HERE'\n", server.GetURL())
	fmt.Println("3. Push a message:")
	fmt.Printf("   curl -X POST %s/v2/instance/push -H 'Cookie: token=YOUR_JWT_HERE' -H 'Content-Type: application/json' -d '{\"UMHMessages\":[{\"metadata\":{\"traceId\":\"12345678-1234-1234-1234-123456789012\"},\"email\":\"test@example.com\",\"content\":\"Hello from curl\",\"umhInstance\":\"12345678-1234-1234-1234-123456789012\"}]}'\n", server.GetURL())

	// Monitor for pushed messages
	go func() {
		for {
			select {
			case msg := <-server.PushedMessages:
				fmt.Printf("Received pushed message: %s from %s\n", msg.Content, msg.Email)
			case <-time.After(10 * time.Second):
				// Just continue monitoring
			}
		}
	}()

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Println("\nShutting down mock server...")
}
