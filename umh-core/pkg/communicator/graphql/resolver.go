package graphql

// THIS CODE WILL BE UPDATED WITH SCHEMA CHANGES. PREVIOUS IMPLEMENTATION FOR SCHEMA CHANGES WILL BE KEPT IN THE COMMENT SECTION. IMPLEMENTATION FOR UNCHANGED SCHEMA WILL BE KEPT.

import (
	"context"
	"strings"
	"time"
)

// SnapshotProvider provides access to system snapshots containing topic data
type SnapshotProvider interface {
	GetSnapshot() *SystemSnapshot
}

// SystemSnapshot represents the system state snapshot
type SystemSnapshot struct {
	Managers map[string]interface{}
}

type Resolver struct {
	SnapshotProvider SnapshotProvider
}

// Topics is the resolver for the topics field.
func (r *queryResolver) Topics(ctx context.Context, filter *TopicFilter, limit *int) ([]*Topic, error) {
	snapshot := r.SnapshotProvider.GetSnapshot()
	if snapshot == nil {
		return []*Topic{}, nil
	}

	// Get topic browser state from snapshot
	topicBrowserState, ok := snapshot.Managers["topic-browser"]
	if !ok {
		return []*Topic{}, nil
	}

	// Extract topics from the state
	// This will be properly typed once we have the real types
	topics := r.extractTopicsFromState(topicBrowserState)

	// Apply filters
	if filter != nil {
		topics = r.filterTopics(topics, filter)
	}

	// Apply limit
	maxLimit := 100
	if limit != nil && *limit > 0 && *limit < maxLimit {
		maxLimit = *limit
	}
	if len(topics) > maxLimit {
		topics = topics[:maxLimit]
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

// Helper methods for topic extraction and filtering
func (r *queryResolver) extractTopicsFromState(state interface{}) []*Topic {
	// For now, return a mock topic to test the structure
	// This will be replaced with actual topic extraction logic
	return []*Topic{
		{
			Topic: "enterprise.site.area.productionline.workstation.sensor.temperature",
			Metadata: []*MetadataKv{
				{Key: "location", Value: "workstation-1"},
				{Key: "sensor-type", Value: "temperature"},
			},
			LastEvent: &TimeSeriesEvent{
				ProducedAt: time.Now(),
				Headers: []*MetadataKv{
					{Key: "source", Value: "opcua-server"},
				},
				SourceTs:     time.Now().Add(-time.Minute),
				ScalarType:   ScalarTypeNumeric,
				NumericValue: func() *float64 { v := 23.5; return &v }(),
			},
		},
	}
}

func (r *queryResolver) filterTopics(topics []*Topic, filter *TopicFilter) []*Topic {
	if filter == nil {
		return topics
	}

	var filtered []*Topic

	for _, topic := range topics {
		match := true

		// Text filter - search in topic name and metadata
		if filter.Text != nil && *filter.Text != "" {
			textMatch := false
			searchText := strings.ToLower(*filter.Text)

			// Search in topic name
			if strings.Contains(strings.ToLower(topic.Topic), searchText) {
				textMatch = true
			}

			// Search in metadata
			if !textMatch {
				for _, meta := range topic.Metadata {
					if strings.Contains(strings.ToLower(meta.Key), searchText) ||
						strings.Contains(strings.ToLower(meta.Value), searchText) {
						textMatch = true
						break
					}
				}
			}

			if !textMatch {
				match = false
			}
		}

		// Metadata filters
		if filter.Meta != nil && match {
			for _, metaFilter := range filter.Meta {
				found := false
				for _, meta := range topic.Metadata {
					if meta.Key == metaFilter.Key {
						if metaFilter.Eq == nil || *metaFilter.Eq == meta.Value {
							found = true
							break
						}
					}
				}
				if !found {
					match = false
					break
				}
			}
		}

		if match {
			filtered = append(filtered, topic)
		}
	}

	return filtered
}

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
