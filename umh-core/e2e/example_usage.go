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

package e2e_test

import (
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// ExampleUsage demonstrates how to use the E2E test infrastructure
func ExampleUsage() {
	// 1. Create and start the mock API server
	mockServer := NewMockAPIServer()
	defer mockServer.Stop()

	err := mockServer.Start()
	if err != nil {
		panic(err)
	}

	// 2. Start UMH Core container pointing to our mock server
	containerName := startUMHCoreWithMockAPI(mockServer)
	defer stopAndRemoveContainer(containerName)

	// 3. Wait for container to be healthy
	for !isContainerHealthy(containerName) {
		time.Sleep(1 * time.Second)
	}

	// 4. Add messages to the pull queue that UMH Core will receive
	testMessage := models.UMHMessage{
		Metadata: &models.MessageMetadata{
			TraceID: uuid.New(),
		},
		Email:        "test@example.com",
		Content:      "Hello from E2E test!",
		InstanceUUID: uuid.New(),
	}

	mockServer.AddMessageToPullQueue(testMessage)

	// 5. Wait for UMH Core to pull and process the message
	time.Sleep(2 * time.Second)

	// 6. Check what UMH Core sent back to us
	pushedMessages := mockServer.DrainPushedMessages()

	// 7. Verify the results
	for i, msg := range pushedMessages {
		println("Received message", i, ":", msg.Content)
	}
}
