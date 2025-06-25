package graphql

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

// MockSnapshotProvider for testing
type MockSnapshotProvider struct{}

func (m *MockSnapshotProvider) GetSnapshot() *SystemSnapshot {
	return &SystemSnapshot{
		Managers: map[string]interface{}{
			"topic-browser": map[string]interface{}{
				"topics": []interface{}{},
			},
		},
	}
}

func TestResolver_Topics(t *testing.T) {
	logger := zap.NewNop()
	resolver := &Resolver{
		SnapshotProvider: &MockSnapshotProvider{},
	}

	queryResolver := &queryResolver{resolver}

	// Test basic topics query
	topics, err := queryResolver.Topics(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Topics query failed: %v", err)
	}

	if len(topics) == 0 {
		t.Error("Expected at least one mock topic, got none")
	}

	// Verify the mock topic structure
	if len(topics) > 0 {
		topic := topics[0]
		if topic.Topic == "" {
			t.Error("Topic name should not be empty")
		}
		if len(topic.Metadata) == 0 {
			t.Error("Expected metadata, got none")
		}
		if topic.LastEvent == nil {
			t.Error("Expected lastEvent, got nil")
		}
	}
}

func TestResolver_Topic(t *testing.T) {
	resolver := &Resolver{
		SnapshotProvider: &MockSnapshotProvider{},
	}

	queryResolver := &queryResolver{resolver}

	// Test single topic query
	topic, err := queryResolver.Topic(context.Background(), "enterprise.site.area.productionline.workstation.sensor.temperature")
	if err != nil {
		t.Fatalf("Topic query failed: %v", err)
	}

	if topic == nil {
		t.Error("Expected to find mock topic, got nil")
	}
}

func TestServer_Creation(t *testing.T) {
	logger := zap.NewNop()
	resolver := &Resolver{
		SnapshotProvider: &MockSnapshotProvider{},
	}

	server := NewServer(resolver, logger)
	if server == nil {
		t.Error("NewServer should not return nil")
	}

	if server.resolver != resolver {
		t.Error("Server should store the provided resolver")
	}

	if server.logger != logger {
		t.Error("Server should store the provided logger")
	}
}
