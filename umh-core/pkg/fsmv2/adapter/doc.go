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

// Package adapter runs an fsmv2 worker behind the fsmv1 FSMInstance / FSMManager
// interfaces, generically, so an fsmv2-backed component drops into the existing
// control loop without a hand-written bridge per worker type.
//
// A WorkerManager reconciles a fleet: it diffs the desired configs pulled from a
// SystemSnapshot against its instance map, Upserts new and changed workers and
// Deletes removed ones through the global fsmv2client, and exposes each as an
// AdaptedInstance. The manager is the whole fsmv1 surface; the developer never
// writes an FSMInstance.
//
// # Framework-owned state resolution
//
// The adapter resolves the fsmv1 state centrally so a missing or wrong developer
// mapping cannot fall through to a false-healthy state. GetCurrentFSMState
// resolves in this precedence; the developer only ever touches the last case:
//
//  1. disabled (desired state stopped)          -> the desired state, no store read
//  2. Unknown (nil client / read hiccup)        -> hold the last known state ("starting" if none)
//  3. degraded verdict (poll error or Health)   -> "degraded"
//  4. Unregistered / NeverObserved (bootstrap)  -> "starting"
//  5. Stale (~3 missed polls)                    -> "degraded"
//  6. Fresh and healthy                          -> the developer's MapFresh
//
// The non-Fresh literals ("starting"/"degraded") are the fleet-wide fsmv1
// lifecycle states every consuming FSM understands; they are the adapter's
// output vocabulary, distinct from a simple worker's own state machine.
//
// The verdict is read off the stored status through the HealthReporter interface,
// so the adapter imports nothing from the worker package and stays generic over
// the stored status type. StaleAfter is 3x the worker type's ObservationInterval
// (1s fallback), derived from the registry, not a developer knob. Because the
// desired state is config-driven, SetDesiredFSMState returns an error and the
// fsmv1 lifecycle methods are no-ops; only Remove actuates (it Deletes the ref).
//
// # The Spec
//
// Five functions are required; four fields default:
//
//	WorkerManagerSpec[TConfig, TStatus]{
//	    WorkerType     string                                                    // required
//	    ExtractConfigs func(SystemSnapshot) []TConfig                            // required
//	    NameOf         func(cfg TConfig) string                                  // required
//	    MapFresh       func(cfg TConfig, status TStatus) string                  // required (Fresh case only)
//	    MapObserved    func(cfg TConfig, status TStatus) publicfsm.ObservedState // required
//	    ConfigEqual    func(a, b TConfig) bool                    // default reflect.DeepEqual
//	    CfgFor         func(cfg TConfig) (map[string]any, error)  // default JSON round-trip
//	    IsEnabled      func(cfg TConfig) bool                     // default from the desired state
//	    MinRequiredTime time.Duration                            // default 0
//	}
//
// TStatus is the STORED status type. For a worker built with package simple that
// is simple.Status[Raw], so MapFresh/MapObserved receive the wrapper and unwrap
// its .Result:
//
//	adapter.NewWorkerManager(adapter.WorkerManagerSpec[Config, simple.Status[Status]]{
//	    WorkerType:     "port_monitor",
//	    ExtractConfigs: func(s publicfsm.SystemSnapshot) []Config { return configsFrom(s) },
//	    NameOf:         func(c Config) string { return c.Address },
//	    MapFresh: func(_ Config, s simple.Status[Status]) string {
//	        if s.Result.Open {
//	            return "open"
//	        }
//	        return "closed"
//	    },
//	    MapObserved: func(_ Config, s simple.Status[Status]) publicfsm.ObservedState {
//	        return ObservedState{Open: s.Result.Open}
//	    },
//	})
//
// The ref is derived internally from {WorkerType, NameOf(cfg)}; instances are
// built by the manager, so there is no caller-supplied constructor.
package adapter
