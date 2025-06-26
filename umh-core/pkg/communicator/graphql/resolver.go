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

import "context"

// This file contains the generated GraphQL resolver interfaces and delegation
// Custom implementation is in resolver_impl.go to maintain separation of concerns

// Resolver struct contains dependencies and delegates to implementation methods
type Resolver struct {
	deps *ResolverDependencies
}

// Query resolver methods - these delegate to the implementation

// Topics is the resolver for the topics field.
func (r *queryResolver) Topics(ctx context.Context, filter *TopicFilter, limit *int) ([]*Topic, error) {
	return r.Resolver.Topics(ctx, filter, limit)
}

// Topic is the resolver for the topic field.
func (r *queryResolver) Topic(ctx context.Context, topic string) (*Topic, error) {
	return r.Resolver.Topic(ctx, topic)
}

// Generated boilerplate required by gqlgen
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
