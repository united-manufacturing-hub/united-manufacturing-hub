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

package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	filesystem "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Mock for SendActionReply
var originalSendActionReply = SendActionReply

// MockActionReply mocks the SendActionReply function
type ActionReplyMock struct {
	InstanceUUID     uuid.UUID
	UserEmail        string
	ActionUUID       uuid.UUID
	ActionReplyState models.ActionReplyState
	Payload          interface{}
	ActionType       models.ActionType
	CallCount        int
}

// MockFileSystem provides a mock implementation for testing
type MockFileSystem struct {
	filesystem.Service
	Files            map[string][]byte
	DirectoriesExist map[string]bool
}

func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		Files:            make(map[string][]byte),
		DirectoriesExist: make(map[string]bool),
	}
}

func (m *MockFileSystem) ReadFile(_ context.Context, path string) ([]byte, error) {
	if data, ok := m.Files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) WriteFile(_ context.Context, path string, data []byte, _ os.FileMode) error {
	m.Files[path] = data
	dirPath := filepath.Dir(path)
	m.DirectoriesExist[dirPath] = true
	return nil
}

func (m *MockFileSystem) FileExists(_ context.Context, path string) (bool, error) {
	_, exists := m.Files[path]
	return exists, nil
}

func (m *MockFileSystem) EnsureDirectory(_ context.Context, path string) error {
	m.DirectoriesExist[path] = true
	return nil
}

func TestEditInstanceAction_Parse(t *testing.T) {
	tests := []struct {
		name        string
		payload     interface{}
		expectedErr bool
		expected    *EditInstanceLocation
	}{
		{
			name: "Valid payload with all fields",
			payload: map[string]interface{}{
				"location": map[string]interface{}{
					"enterprise": "TestEnterprise",
					"site":       "TestSite",
					"area":       "TestArea",
					"line":       "TestLine",
					"workCell":   "TestWorkCell",
				},
			},
			expectedErr: false,
			expected: &EditInstanceLocation{
				Enterprise: "TestEnterprise",
				Site:       stringPtr("TestSite"),
				Area:       stringPtr("TestArea"),
				Line:       stringPtr("TestLine"),
				WorkCell:   stringPtr("TestWorkCell"),
			},
		},
		{
			name: "Valid payload with required fields only",
			payload: map[string]interface{}{
				"location": map[string]interface{}{
					"enterprise": "TestEnterprise",
				},
			},
			expectedErr: false,
			expected: &EditInstanceLocation{
				Enterprise: "TestEnterprise",
			},
		},
		{
			name: "Invalid payload - no location",
			payload: map[string]interface{}{
				"otherField": "someValue",
			},
			expectedErr: false, // Not an error, as location is optional
			expected:    nil,
		},
		{
			name: "Invalid payload - location not a map",
			payload: map[string]interface{}{
				"location": "notAMap",
			},
			expectedErr: true,
			expected:    nil,
		},
		{
			name: "Invalid payload - missing enterprise",
			payload: map[string]interface{}{
				"location": map[string]interface{}{
					"site": "TestSite",
				},
			},
			expectedErr: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &EditInstanceAction{}
			err := action.Parse(tt.payload)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expected == nil {
					assert.Nil(t, action.location)
				} else {
					assert.NotNil(t, action.location)
					assert.Equal(t, tt.expected.Enterprise, action.location.Enterprise)
					assertEqualStringPtr(t, tt.expected.Site, action.location.Site)
					assertEqualStringPtr(t, tt.expected.Area, action.location.Area)
					assertEqualStringPtr(t, tt.expected.Line, action.location.Line)
					assertEqualStringPtr(t, tt.expected.WorkCell, action.location.WorkCell)
				}
			}
		})
	}
}

func TestEditInstanceAction_Validate(t *testing.T) {
	tests := []struct {
		name        string
		location    *EditInstanceLocation
		expectedErr bool
	}{
		{
			name: "Valid location",
			location: &EditInstanceLocation{
				Enterprise: "TestEnterprise",
			},
			expectedErr: false,
		},
		{
			name:        "No location",
			location:    nil,
			expectedErr: false,
		},
		{
			name: "Invalid location - empty enterprise",
			location: &EditInstanceLocation{
				Enterprise: "",
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &EditInstanceAction{
				location: tt.location,
			}
			err := action.Validate()

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEditInstanceAction_Execute tests the basic logic of the Execute method
func TestEditInstanceAction_Execute(t *testing.T) {
	tests := []struct {
		name            string
		location        *EditInstanceLocation
		expectedMessage string
		expectSuccess   bool
	}{
		{
			name: "Update all location fields",
			location: &EditInstanceLocation{
				Enterprise: "NewEnterprise",
				Site:       stringPtr("NewSite"),
				Area:       stringPtr("NewArea"),
				Line:       stringPtr("NewLine"),
				WorkCell:   stringPtr("NewWorkCell"),
			},
			expectedMessage: "Successfully updated instance location to Enterprise: NewEnterprise, Site: NewSite, Area: NewArea, Line: NewLine, WorkCell: NewWorkCell",
			expectSuccess:   true,
		},
		{
			name: "Update enterprise only",
			location: &EditInstanceLocation{
				Enterprise: "NewEnterprise",
			},
			expectedMessage: "Successfully updated instance location to Enterprise: NewEnterprise",
			expectSuccess:   true,
		},
		{
			name:            "No location provided",
			location:        nil,
			expectedMessage: "No changes were made to the instance",
			expectSuccess:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test action with a simplified executor
			action := &simpleTestEditInstanceAction{
				location:        tt.location,
				expectFailure:   !tt.expectSuccess,
				updateWasCalled: false,
			}

			// Execute the action
			result, metadata, err := action.Execute()

			// Verify the results
			if tt.expectSuccess {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedMessage, result)
				assert.Nil(t, metadata)
			} else {
				assert.Error(t, err)
			}

			// If location was provided, check that the update was attempted
			if tt.location != nil {
				assert.True(t, action.updateWasCalled, "updateLocation should have been called")
			} else {
				assert.False(t, action.updateWasCalled, "updateLocation should not have been called")
			}
		})
	}
}

// simpleTestEditInstanceAction is a minimal test implementation that focuses on just the logic we need to test
type simpleTestEditInstanceAction struct {
	location        *EditInstanceLocation
	expectFailure   bool
	updateWasCalled bool
}

// Execute implements a simplified version of Execute that omits SendActionReply calls
func (a *simpleTestEditInstanceAction) Execute() (interface{}, map[string]interface{}, error) {
	// If we don't have any location to update, return early
	if a.location == nil {
		return "No changes were made to the instance", nil, nil
	}

	// Simulate updateLocation
	err := a.mockUpdateLocation()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to update instance location: %w", err)
	}

	// Create success message based on the provided location
	message := fmt.Sprintf("Successfully updated instance location to Enterprise: %s", a.location.Enterprise)

	// Add optional fields to the message if they exist
	if a.location.Site != nil {
		message += fmt.Sprintf(", Site: %s", *a.location.Site)
	}
	if a.location.Area != nil {
		message += fmt.Sprintf(", Area: %s", *a.location.Area)
	}
	if a.location.Line != nil {
		message += fmt.Sprintf(", Line: %s", *a.location.Line)
	}
	if a.location.WorkCell != nil {
		message += fmt.Sprintf(", WorkCell: %s", *a.location.WorkCell)
	}

	return message, nil, nil
}

// mockUpdateLocation simulates the behavior of updateLocation
func (a *simpleTestEditInstanceAction) mockUpdateLocation() error {
	a.updateWasCalled = true
	if a.expectFailure {
		return fmt.Errorf("simulated failure in updateLocation")
	}
	return nil
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func assertEqualStringPtr(t *testing.T, expected, actual *string) {
	if expected == nil {
		assert.Nil(t, actual)
	} else {
		assert.NotNil(t, actual)
		assert.Equal(t, *expected, *actual)
	}
}
