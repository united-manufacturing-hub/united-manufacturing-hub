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

// Package simple lets a developer write a polling monitor worker in one file.
//
// A monitor worker only polls something and reports health. simple removes the
// boilerplate that shape otherwise costs (a config, a deps struct, a worker, an
// action, and a handful of state files): the developer fills a struct-literal
// MonitorSpec and registers it once from an init(). The framework owns the state
// machine, the collection cadence, and the health-verdict resolution.
//
// # The MonitorSpec
//
// Two fields are required, three are optional:
//
//	MonitorSpec[TConfig, TStatus, TDeps]{
//	    WorkerType string                                                       // required
//	    Poll       func(ctx, d TDeps, cfg TConfig) (TStatus, error)             // required
//	    Health     func(cfg TConfig, status TStatus) Health                     // optional
//	    NewDeps    func(id deps.Identity, bd *deps.BaseDependencies) TDeps      // optional (built once per instance)
//	    Interval   time.Duration                                               // optional (collector default if 0)
//	}
//
// A poll that needs no dependencies instantiates TDeps as struct{} and leaves
// NewDeps unset, so Poll receives the zero value. NewDeps is handed the
// framework's BaseDependencies for the instance, so a dependency value that
// needs the worker's logger takes it from there rather than from a package-level
// logger. Poll takes TDeps by value, so state it mutates has to sit behind a
// pointer.
//
// TStatus must be a struct (Register panics otherwise): the framework flattens
// it to top-level JSON for CSE delta sync.
//
// # Resources in the deps value
//
// The framework never releases what NewDeps returns. A despawned worker is
// dropped without any teardown call, so the value must be safe to abandon: a
// buffer or a counter is, a connection pool or anything else holding a
// background goroutine is not.
//
// To share one such resource across instances, declare it at package level and
// close over it in NewDeps; anything constructed inside the builder body is per
// instance. Share it only when the worker type is a singleton, or when the
// resource does not depend on per-instance config. A single-slot cache keyed by
// config holds one resource at a time, so once two instances disagree on the key
// every poll evicts the other's and rebuilds its own.
//
// A multi-instance worker whose instances differ on the key needs a keyed
// package-level registry instead: a map[string]*resource guarded by a mutex, one
// entry per distinct key. It never thrashes, and it bounds retention at one
// resource per key ever configured. Package historian is a worked example of the
// singleton case.
//
// # Framework telemetry follows BaseDependencies
//
// The framework attaches its metrics to a simple worker's Observation only when
// the deps value it bound carries the BaseDependencies accessors, so only when
// TDeps embeds *deps.BaseDependencies and NewDeps supplies it. The collector
// reads the framework fields off that value; a worker declaring
// TDeps = struct{} gets no framework metrics, and no error saying so. Package
// historian declares such a NewDeps and is injected; the port monitor in the
// Example below declares none and is not. Action history travels the same path,
// and a simple worker dispatches no actions, so it stays empty either way.
//
// Both stay in CSE: a status generator and the fsmv1 adapter see the Status
// alone. Worker metrics are the exception that leaves the process. Counters and
// gauges recorded through bd.MetricsRecorder() are exported to the agent's
// Prometheus /metrics endpoint, labelled by hierarchy path.
//
// # Two-phase Poll then Health
//
// Every tick the framework runs Poll first. On a Poll error the worker is
// degraded with reason "poll error: <err>" and Health is NOT called — the error
// is persisted as a verdict, not returned, so the worker reports degraded with a
// reason instead of hanging in a bootstrap state. On a good poll the optional
// Health function decides the verdict; when it is nil the worker is healthy with
// reason "running (no health check)".
//
// Status[TStatus] holds the verdict (Result + Degraded + Reason); the framework
// sets it on the observation. The state machine reads it to switch between
// running and degraded (emitting the reason on each Transition); the fsmv1
// adapter reads it through the HealthReporter interface Status satisfies. Nothing
// is added to the shared fsmv2.Observation API.
//
// # Example
//
//	package portmonitor
//
//	import (
//	    "context"
//	    "net"
//	    "time"
//
//	    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
//	)
//
//	type Config struct {
//	    Address string `json:"address"` // e.g. "192.0.2.1:502"
//	}
//
//	type Status struct {
//	    Open bool `json:"open"`
//	}
//
//	func init() {
//	    simple.Register(simple.MonitorSpec[Config, Status, struct{}]{
//	        WorkerType: "port_monitor",
//	        Interval:   5 * time.Second,
//	        Poll: func(ctx context.Context, _ struct{}, cfg Config) (Status, error) {
//	            d := net.Dialer{}
//	            conn, err := d.DialContext(ctx, "tcp", cfg.Address)
//	            if err != nil {
//	                return Status{}, err // -> degraded, "poll error: <err>"
//	            }
//	            conn.Close()
//	            return Status{Open: true}, nil
//	        },
//	        Health: func(_ Config, s Status) simple.Health {
//	            if !s.Open {
//	                return simple.Degraded("port closed")
//	            }
//	            return simple.Healthy("port open")
//	        },
//	    })
//	}
//
// To expose the worker behind the fsmv1 control loop, pair it with an
// adapter.WorkerManager whose TStatus is simple.Status[Status]; see package
// adapter.
package simple
