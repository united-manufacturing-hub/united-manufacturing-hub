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

package graphql

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"go.uber.org/zap"
)

// StartGraphQLServer is a convenience function for main.go to start the GraphQL server
// It handles the configuration conversion and server creation
func StartGraphQLServer(
	resolver *Resolver,
	cfg *config.GraphQLConfig,
	logger *zap.SugaredLogger,
) (*Server, error) {
	if !cfg.Enabled {
		return nil, nil // Server not enabled, return nil without error
	}

	// Convert config
	adapter := NewGraphQLConfigAdapter(cfg)
	serverConfig := NewServerConfigFromAdapter(adapter)

	// Create and start server
	server, err := NewServer(resolver, serverConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL server: %w", err)
	}

	// Start the server
	if err := server.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start GraphQL server: %w", err)
	}

	return server, nil
}

// buildTopicName constructs the full topic path from protobuf topic info
// Uses the expert's exact algorithm: Level0 + LocationSublevels + DataContract + VirtualPath + Name
func buildTopicName(topicInfo *tbproto.TopicInfo) string {
	var parts []string
	parts = append(parts, topicInfo.Level0)
	parts = append(parts, topicInfo.LocationSublevels...)
	parts = append(parts, topicInfo.DataContract)

	// VirtualPath may already contain dots; add as-is if present
	if topicInfo.VirtualPath != nil && *topicInfo.VirtualPath != "" {
		parts = append(parts, *topicInfo.VirtualPath)
	}

	parts = append(parts, topicInfo.Name)

	return strings.Join(parts, ".")
}

// hashUNSTableEntry generates an xxHash from the Levels and datacontract.
// This is used by the frontend to identify which topic an entry belongs to.
// We use it over full topic names to reduce the amount of data we need to send to the frontend.
//
// NOTE: This function is copied from the benthos topic browser plugin to ensure
// identical hash generation for compatibility across components.
//
// ✅ FIX: Uses null byte delimiters to prevent hash collisions between different segment combinations.
// For example, ["ab","c"] vs ["a","bc"] would produce different hashes instead of identical ones.
func hashUNSTableEntry(info *tbproto.TopicInfo) string {
	hasher := xxhash.New()

	// Helper function to write each component followed by NUL delimiter to avoid ambiguity
	write := func(s string) {
		_, _ = hasher.Write(append([]byte(s), 0))
	}

	write(info.Level0)

	// Hash all location sublevels
	for _, level := range info.LocationSublevels {
		write(level)
	}

	write(info.DataContract)

	// Hash virtual path if it exists
	if info.VirtualPath != nil {
		write(*info.VirtualPath)
	}

	// Hash the name (new field)
	write(info.Name)

	return hex.EncodeToString(hasher.Sum(nil))
}

// mapMetadataToGraphQL converts protobuf metadata to GraphQL metadata format
func mapMetadataToGraphQL(metadata map[string]string) []*MetadataKv {
	var result []*MetadataKv
	for key, value := range metadata {
		result = append(result, &MetadataKv{
			Key:   key,
			Value: value,
		})
	}
	return result
}

// mapEventEntryToGraphQL converts protobuf event entry to GraphQL event format
func mapEventEntryToGraphQL(entry *tbproto.EventTableEntry) Event {
	timestamp := time.UnixMilli(int64(entry.ProducedAtMs))

	// Determine event type based on payload format
	switch entry.PayloadFormat {
	case tbproto.PayloadFormat_TIMESERIES:
		return mapTimeSeriesEvent(entry, timestamp)
	case tbproto.PayloadFormat_RELATIONAL:
		return mapRelationalEvent(entry, timestamp)
	default:
		// Default to time series for unknown formats
		return mapTimeSeriesEvent(entry, timestamp)
	}
}

// mapTimeSeriesEvent converts protobuf time series event to GraphQL time series event
func mapTimeSeriesEvent(entry *tbproto.EventTableEntry, timestamp time.Time) *TimeSeriesEvent {
	headers := mapKafkaHeaders(entry.RawKafkaMsg)

	// Get the time series payload
	tsPayload := entry.GetTs()
	if tsPayload == nil {
		// Return empty time series event if no payload
		return &TimeSeriesEvent{
			ProducedAt:  timestamp,
			Headers:     headers,
			SourceTs:    timestamp,
			ScalarType:  ScalarTypeString,
			StringValue: nil,
		}
	}

	var scalarType ScalarType
	var numericValue *float64
	var stringValue *string
	var booleanValue *bool
	var sourceTs time.Time

	if tsPayload.TimestampMs > 0 {
		sourceTs = time.UnixMilli(tsPayload.TimestampMs)
	} else {
		sourceTs = timestamp
	}

	// Extract scalar value based on type
	switch tsPayload.ScalarType {
	case tbproto.ScalarType_BOOLEAN:
		scalarType = ScalarTypeBoolean
		if boolVal := tsPayload.GetBooleanValue(); boolVal != nil {
			value := boolVal.Value
			booleanValue = &value
		}
	case tbproto.ScalarType_STRING:
		scalarType = ScalarTypeString
		if strVal := tsPayload.GetStringValue(); strVal != nil {
			value := strVal.Value
			stringValue = &value
		}
	case tbproto.ScalarType_NUMERIC:
		scalarType = ScalarTypeNumeric
		if numVal := tsPayload.GetNumericValue(); numVal != nil {
			value := numVal.Value
			numericValue = &value
		}
	default:
		scalarType = ScalarTypeString
		emptyStr := ""
		stringValue = &emptyStr
	}

	return &TimeSeriesEvent{
		ProducedAt:   timestamp,
		Headers:      headers,
		SourceTs:     sourceTs,
		ScalarType:   scalarType,
		NumericValue: numericValue,
		StringValue:  stringValue,
		BooleanValue: booleanValue,
	}
}

// mapRelationalEvent converts protobuf relational event to GraphQL relational event
func mapRelationalEvent(entry *tbproto.EventTableEntry, timestamp time.Time) *RelationalEvent {
	headers := mapKafkaHeaders(entry.RawKafkaMsg)

	// Get the relational payload
	relPayload := entry.GetRel()
	var jsonData map[string]any

	if relPayload != nil && len(relPayload.Json) > 0 {
		// Parse the JSON payload
		if err := json.Unmarshal(relPayload.Json, &jsonData); err != nil {
			// Log the error for debugging/monitoring (with context)
			log := logger.For(logger.ComponentCommunicator)
			log.Warnw("Failed to unmarshal relational event JSON in GraphQL resolver",
				"error", err,
				"payload_size", len(relPayload.Json),
				"uns_tree_id", entry.UnsTreeId,
				"produced_at_ms", entry.ProducedAtMs,
			)

			// Create object with parse error indicator for debugging
			jsonData = map[string]any{
				"_parseError": err.Error(),
				"_rawSize":    len(relPayload.Json),
			}
		}
	} else {
		jsonData = make(map[string]any)
	}

	return &RelationalEvent{
		ProducedAt: timestamp,
		Headers:    headers,
		JSON:       jsonData,
	}
}

// mapKafkaHeaders converts protobuf Kafka headers to GraphQL metadata format
func mapKafkaHeaders(eventKafka *tbproto.EventKafka) []*MetadataKv {
	var headers []*MetadataKv
	if eventKafka != nil && eventKafka.Headers != nil {
		for key, value := range eventKafka.Headers {
			headers = append(headers, &MetadataKv{
				Key:   key,
				Value: value,
			})
		}
	}
	return headers
}

// matchesFilter checks if a topic matches the provided filter criteria
func matchesFilter(topic *Topic, filter *TopicFilter) bool {
	// Text filter - search in topic path AND metadata values as per expert feedback
	if filter.Text != nil && *filter.Text != "" {
		searchText := strings.ToLower(*filter.Text)

		// Search in topic path
		if strings.Contains(strings.ToLower(topic.Topic), searchText) {
			// Found in topic path, continue to metadata filter check
		} else {
			// Not found in topic path, check metadata
			foundInMetadata := false
			for _, kv := range topic.Metadata {
				if strings.Contains(strings.ToLower(kv.Key), searchText) ||
					strings.Contains(strings.ToLower(kv.Value), searchText) {
					foundInMetadata = true
					break
				}
			}
			if !foundInMetadata {
				return false
			}
		}
	}

	// Metadata filter (exact key-value matching)
	if len(filter.Meta) > 0 {
		if !matchesMetadataFilter(topic.Metadata, filter.Meta) {
			return false
		}
	}

	return true
}

// matchesMetadataFilter checks if topic metadata matches the filter metadata expressions
func matchesMetadataFilter(topicMetadata []*MetadataKv, filterMetadata []*MetaExpr) bool {
	metadataMap := make(map[string]string)
	for _, kv := range topicMetadata {
		metadataMap[kv.Key] = kv.Value
	}

	for _, metaFilter := range filterMetadata {
		value, exists := metadataMap[metaFilter.Key]
		if !exists {
			return false
		}

		if metaFilter.Eq != nil && *metaFilter.Eq != value {
			return false
		}
	}

	return true
}
