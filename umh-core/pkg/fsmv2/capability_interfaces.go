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

package fsmv2

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ActionProvider enables side effects via actions.
// Workers that implement this interface opt into the action execution pipeline.
// The supervisor calls Actions() once at registration to discover available actions.
type ActionProvider interface {
	Actions() map[string]Action[any]
}

// ChildSpecProvider enables parent-child orchestration.
// Workers that implement this interface become parents. The supervisor
// creates and manages child workers based on the returned specifications.
type ChildSpecProvider interface {
	ChildSpecs() []config.ChildSpec
}

// MetricsProvider enables custom Prometheus metrics.
// Workers that implement this interface register custom collectors
// with Prometheus at registration time.
type MetricsProvider interface {
	Metrics() []prometheus.Collector
}

// GracefulShutdowner enables custom cleanup on shutdown.
// Workers that implement this interface get a chance to flush buffers,
// close connections, etc. before the supervisor removes the worker.
type GracefulShutdowner interface {
	Shutdown(ctx context.Context) error
}

// ChildrenViewConsumer enables access to the full child state tree.
// Workers that implement this interface receive the complete children
// supervisor view each tick, enabling extraction of circuit breaker state,
// stale counts, and other detailed child information beyond aggregate counts.
type ChildrenViewConsumer interface {
	SetChildrenView(view any)
}
