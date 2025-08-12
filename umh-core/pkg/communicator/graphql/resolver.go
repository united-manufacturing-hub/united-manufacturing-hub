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

//go:generate go run github.com/99designs/gqlgen generate

package graphql

// THIS CODE WILL BE UPDATED WITH SCHEMA CHANGES. PREVIOUS IMPLEMENTATION FOR SCHEMA CHANGES WILL BE KEPT IN THE COMMENT SECTION. IMPLEMENTATION FOR UNCHANGED SCHEMA WILL BE KEPT.

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

// SnapshotProvider provides access to system snapshots containing topic data.
type SnapshotProvider interface {
	GetSnapshot() *SystemSnapshot
}

// SystemSnapshot represents the system state snapshot.
type SystemSnapshot struct {
	Managers map[string]interface{}
}

// TopicBrowserCacheInterface defines the interface for topic browser cache.
type TopicBrowserCacheInterface interface {
	GetEventMap() map[string]*tbproto.EventTableEntry
	GetUnsMap() *tbproto.TopicMap
}

type Resolver struct {
	SnapshotManager   *fsm.SnapshotManager
	TopicBrowserCache TopicBrowserCacheInterface
}

// Topics is the resolver for the topics field.
func (r *queryResolver) Topics(ctx context.Context, filter *TopicFilter, limit *int) ([]*Topic, error) {
	if r.TopicBrowserCache == nil {
		return []*Topic{}, nil
	}

	// Get snapshot of cached event data
	eventMap := r.TopicBrowserCache.GetEventMap()
	unsMap := r.TopicBrowserCache.GetUnsMap()

	if unsMap == nil || unsMap.Entries == nil {
		return []*Topic{}, nil
	}

	// Determine effective limit early
	const defaultMaxLimit = 100

	maxLimit := defaultMaxLimit
	if limit != nil && *limit > 0 && *limit < defaultMaxLimit {
		maxLimit = *limit
	}

	var topics []*Topic

	// Convert protobuf data to GraphQL models with early termination
	processedCount := 0
	for _, topicInfo := range unsMap.GetEntries() {
		// Check context cancellation for long-running operations
		if processedCount%50 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Calculate the hashed UNS tree ID for this topic (same as simulator)
		hashedTreeId := r.hashUNSTableEntry(topicInfo)

		// Create topic object
		topic := &Topic{
			Topic:     r.buildTopicName(topicInfo),
			Metadata:  r.mapMetadataToGraphQL(topicInfo.GetMetadata()),
			LastEvent: nil, // Will be populated below if event exists
		}

		// Apply filters early to avoid unnecessary processing
		if filter != nil && !r.matchesFilter(topic, filter) {
			processedCount++

			continue
		}

		// Get latest event for this topic if available (only after filtering)
		if eventEntry, exists := eventMap[hashedTreeId]; exists {
			topic.LastEvent = r.mapEventEntryToGraphQL(eventEntry)
		}

		topics = append(topics, topic)

		// Stop processing if we've reached the limit (after filtering)
		if len(topics) >= maxLimit {
			break
		}

		processedCount++
	}

	return topics, nil
}

// Topic is the resolver for the topic field.
func (r *queryResolver) Topic(ctx context.Context, topic string) (*Topic, error) {
	topics, err := r.Topics(ctx, &TopicFilter{Text: &topic}, nil)
	if err != nil {
		return nil, err
	}

	for _, t := range topics {
		if t.Topic == topic {
			return t, nil
		}
	}

	return nil, nil
}

// Helper methods for data conversion and filtering

func (r *Resolver) buildTopicName(topicInfo *tbproto.TopicInfo) string {
	// Use expert's exact algorithm: Level0 + LocationSublevels + DataContract + VirtualPath + Name
	var parts []string

	parts = append(parts, topicInfo.GetLevel0())
	parts = append(parts, topicInfo.GetLocationSublevels()...)
	parts = append(parts, topicInfo.GetDataContract())

	// VirtualPath may already contain dots; add as-is if present
	if topicInfo.VirtualPath != nil && topicInfo.GetVirtualPath() != "" {
		parts = append(parts, topicInfo.GetVirtualPath())
	}

	parts = append(parts, topicInfo.GetName())

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
func (r *Resolver) hashUNSTableEntry(info *tbproto.TopicInfo) string {
	hasher := xxhash.New()

	// Helper function to write each component followed by NUL delimiter to avoid ambiguity
	write := func(s string) {
		_, _ = hasher.Write(append([]byte(s), 0))
	}

	write(info.GetLevel0())

	// Hash all location sublevels
	for _, level := range info.GetLocationSublevels() {
		write(level)
	}

	write(info.GetDataContract())

	// Hash virtual path if it exists
	if info.VirtualPath != nil {
		write(info.GetVirtualPath())
	}

	// Hash the name (new field)
	write(info.GetName())

	return hex.EncodeToString(hasher.Sum(nil))
}

func (r *Resolver) mapMetadataToGraphQL(metadata map[string]string) []*MetadataKv {
	var result []*MetadataKv
	for key, value := range metadata {
		result = append(result, &MetadataKv{
			Key:   key,
			Value: value,
		})
	}

	return result
}

func (r *Resolver) mapEventEntryToGraphQL(entry *tbproto.EventTableEntry) Event {
	timestamp := time.UnixMilli(int64(entry.GetProducedAtMs()))

	// Determine event type based on payload format
	switch entry.GetPayloadFormat() {
	case tbproto.PayloadFormat_TIMESERIES:
		return r.mapTimeSeriesEvent(entry, timestamp)
	case tbproto.PayloadFormat_RELATIONAL:
		return r.mapRelationalEvent(entry, timestamp)
	default:
		// Default to time series for unknown formats
		return r.mapTimeSeriesEvent(entry, timestamp)
	}
}

func (r *Resolver) mapTimeSeriesEvent(entry *tbproto.EventTableEntry, timestamp time.Time) *TimeSeriesEvent {
	headers := r.mapKafkaHeaders(entry.GetRawKafkaMsg())

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

	var (
		scalarType   ScalarType
		numericValue *float64
		stringValue  *string
		booleanValue *bool
		sourceTs     time.Time
	)

	if tsPayload.GetTimestampMs() > 0 {
		sourceTs = time.UnixMilli(tsPayload.GetTimestampMs())
	} else {
		sourceTs = timestamp
	}

	// Extract scalar value based on type
	switch tsPayload.GetScalarType() {
	case tbproto.ScalarType_BOOLEAN:
		scalarType = ScalarTypeBoolean

		if boolVal := tsPayload.GetBooleanValue(); boolVal != nil {
			value := boolVal.GetValue()
			booleanValue = &value
		}
	case tbproto.ScalarType_STRING:
		scalarType = ScalarTypeString

		if strVal := tsPayload.GetStringValue(); strVal != nil {
			value := strVal.GetValue()
			stringValue = &value
		}
	case tbproto.ScalarType_NUMERIC:
		scalarType = ScalarTypeNumeric

		if numVal := tsPayload.GetNumericValue(); numVal != nil {
			value := numVal.GetValue()
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

func (r *Resolver) mapRelationalEvent(entry *tbproto.EventTableEntry, timestamp time.Time) *RelationalEvent {
	headers := r.mapKafkaHeaders(entry.GetRawKafkaMsg())

	// Get the relational payload
	relPayload := entry.GetRel()

	var jsonData map[string]any

	if relPayload != nil && len(relPayload.GetJson()) > 0 {
		// Parse the JSON payload
		if err := json.Unmarshal(relPayload.GetJson(), &jsonData); err != nil {
			// Log the error for debugging/monitoring (with context)
			log := logger.For(logger.ComponentCommunicator)
			log.Warnw("Failed to unmarshal relational event JSON in GraphQL resolver",
				"error", err,
				"payload_size", len(relPayload.GetJson()),
				"uns_tree_id", entry.GetUnsTreeId(),
				"produced_at_ms", entry.GetProducedAtMs(),
			)

			// Create object with parse error indicator for debugging
			jsonData = map[string]any{
				"_parseError": err.Error(),
				"_rawSize":    len(relPayload.GetJson()),
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

func (r *Resolver) mapKafkaHeaders(eventKafka *tbproto.EventKafka) []*MetadataKv {
	var headers []*MetadataKv

	if eventKafka != nil && eventKafka.Headers != nil {
		for key, value := range eventKafka.GetHeaders() {
			headers = append(headers, &MetadataKv{
				Key:   key,
				Value: value,
			})
		}
	}

	return headers
}

func (r *Resolver) matchesFilter(topic *Topic, filter *TopicFilter) bool {
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
		if !r.matchesMetadataFilter(topic.Metadata, filter.Meta) {
			return false
		}
	}

	return true
}

func (r *Resolver) matchesMetadataFilter(topicMetadata []*MetadataKv, filterMetadata []*MetaExpr) bool {
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

// Query returns QueryResolver which is required by gqlgen.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
