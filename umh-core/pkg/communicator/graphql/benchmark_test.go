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
	"testing"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
)

func BenchmarkResolver_Topics(b *testing.B) {
	resolver := &Resolver{
		TopicBrowserCache: NewMockTopicBrowserCache(),
	}
	queryResolver := &queryResolver{resolver}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := queryResolver.Topics(context.Background(), nil, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolver_TopicsWithTextFilter(b *testing.B) {
	resolver := &Resolver{
		TopicBrowserCache: NewMockTopicBrowserCache(),
	}
	queryResolver := &queryResolver{resolver}
	filter := &TopicFilter{Text: benchStringPtr("temperature")}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := queryResolver.Topics(context.Background(), filter, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolver_TopicsWithLimit(b *testing.B) {
	resolver := &Resolver{
		TopicBrowserCache: NewMockTopicBrowserCache(),
	}
	queryResolver := &queryResolver{resolver}
	limit := benchIntPtr(10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := queryResolver.Topics(context.Background(), nil, limit)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolver_TopicsWithMetaFilter(b *testing.B) {
	resolver := &Resolver{
		TopicBrowserCache: NewMockTopicBrowserCache(),
	}
	queryResolver := &queryResolver{resolver}
	filter := &TopicFilter{
		Meta: []*MetaExpr{
			{Key: "unit", Eq: benchStringPtr("celsius")},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := queryResolver.Topics(context.Background(), filter, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolver_SingleTopic(b *testing.B) {
	resolver := &Resolver{
		TopicBrowserCache: NewMockTopicBrowserCache(),
	}
	queryResolver := &queryResolver{resolver}
	topic := "enterprise.site.area.productionLine.workstation.sensor.temperature"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := queryResolver.Topic(context.Background(), topic)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark the topic hashing function which is critical for performance
func BenchmarkTopicHashing(b *testing.B) {
	resolver := &Resolver{
		TopicBrowserCache: NewMockTopicBrowserCache(),
	}
	topicInfo := &tbproto.TopicInfo{
		Level0:            "enterprise",
		LocationSublevels: []string{"site", "area", "productionLine", "workstation"},
		DataContract:      "sensor",
		Name:              "temperature",
		Metadata:          map[string]string{"unit": "celsius"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = resolver.hashUNSTableEntry(topicInfo)
	}
}

// Benchmark filtering logic
func BenchmarkMatchesFilter(b *testing.B) {
	resolver := &Resolver{
		TopicBrowserCache: NewMockTopicBrowserCache(),
	}

	topic := &Topic{
		Topic: "enterprise.site.area.productionLine.workstation.sensor.temperature",
		Metadata: []*MetadataKv{
			{Key: "unit", Value: "celsius"},
			{Key: "type", Value: "sensor"},
		},
	}

	filter := &TopicFilter{
		Text: benchStringPtr("temperature"),
		Meta: []*MetaExpr{
			{Key: "unit", Eq: benchStringPtr("celsius")},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = resolver.matchesFilter(topic, filter)
	}
}

// Benchmark building topic names from protobuf data
func BenchmarkBuildTopicName(b *testing.B) {
	resolver := &Resolver{
		TopicBrowserCache: NewMockTopicBrowserCache(),
	}
	topicInfo := &tbproto.TopicInfo{
		Level0:            "enterprise",
		LocationSublevels: []string{"site", "area", "productionLine", "workstation"},
		DataContract:      "sensor",
		Name:              "temperature",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = resolver.buildTopicName(topicInfo)
	}
}

// Helper functions for benchmark tests (prefixed to avoid conflicts)
func benchStringPtr(s string) *string { return &s }
func benchIntPtr(i int) *int          { return &i }
