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
//	    Deps       TDeps                                                        // optional (struct{} if none)
//	    Interval   time.Duration                                               // optional (collector default if 0)
//	}
//
// TStatus must be a struct (Register panics otherwise): the framework flattens
// it to top-level JSON for CSE delta sync.
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
