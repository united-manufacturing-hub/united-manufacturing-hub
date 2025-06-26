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
)

// NewResolver creates a new GraphQL resolver with the provided dependencies
// This constructor ensures all required dependencies are properly injected
func NewResolver(deps *ResolverDependencies) *Resolver {
	if deps == nil {
		deps = &ResolverDependencies{} // Provide safe defaults
	}

	return &Resolver{
		deps: deps,
	}
}

// Topics resolves the GraphQL topics query with filtering and pagination
// This is the main entry point for browsing unified namespace topics
func (r *Resolver) Topics(ctx context.Context, filter *TopicFilter, limit *int) ([]*Topic, error) {
	// Early return if no cache available
	if r.deps == nil || r.deps.TopicBrowserCache == nil {
		return []*Topic{}, nil
	}

	// Get snapshot of cached event data
	eventMap := r.deps.TopicBrowserCache.GetEventMap()
	unsMap := r.deps.TopicBrowserCache.GetUnsMap()

	if unsMap == nil || unsMap.Entries == nil {
		return []*Topic{}, nil
	}

	// Determine effective limit early to optimize processing
	const defaultMaxLimit = 100
	maxLimit := defaultMaxLimit
	if limit != nil && *limit > 0 && *limit < defaultMaxLimit {
		maxLimit = *limit
	}

	var topics []*Topic

	// Convert protobuf data to GraphQL models with early termination for performance
	processedCount := 0
	for _, topicInfo := range unsMap.Entries {
		// Check context cancellation for long-running operations
		if processedCount%50 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Calculate the hashed UNS tree ID for this topic (matches simulator algorithm)
		hashedTreeId := hashUNSTableEntry(topicInfo)

		// Create topic object with metadata
		topic := &Topic{
			Topic:     buildTopicName(topicInfo),
			Metadata:  mapMetadataToGraphQL(topicInfo.Metadata),
			LastEvent: nil, // Will be populated below if event exists
		}

		// Apply filters early to avoid unnecessary event processing
		if filter != nil && !matchesFilter(topic, filter) {
			processedCount++
			continue
		}

		// Get latest event for this topic if available (only after filtering for performance)
		if eventEntry, exists := eventMap[hashedTreeId]; exists {
			topic.LastEvent = mapEventEntryToGraphQL(eventEntry)
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

// Topic resolves the GraphQL topic query for a specific topic path
// This provides detailed information about a single topic
func (r *Resolver) Topic(ctx context.Context, topic string) (*Topic, error) {
	// Reuse the Topics resolver with a text filter for the specific topic
	topics, err := r.Topics(ctx, &TopicFilter{Text: &topic}, nil)
	if err != nil {
		return nil, err
	}

	// Find exact match for the requested topic
	for _, t := range topics {
		if t.Topic == topic {
			return t, nil
		}
	}

	// Return nil if topic not found (GraphQL will return null)
	return nil, nil
}
