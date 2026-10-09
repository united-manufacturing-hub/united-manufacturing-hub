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

// Package config provides configuration types for FSMv2 workers.
//
// # Overview
//
// The config package defines:
//   - UserSpec: The raw user configuration for one worker entry
//   - ChildSpec: Declaration of child workers for hierarchical composition
//   - DesiredState: Target state with shutdown control
//   - Template utilities for variable expansion
//
// # Variables
//
// A worker entry's variables are one map: UserSpec.Variables
// (map[string]any) holds the entry's own `variables:` section, decoded as
// written. A template reads one as {{ .variables.<key> }}.
//
// Nothing merges a parent's variables into a child's. A parent that wants a
// child to have a value writes it into the child's ChildSpec (see
// workers/example/exampleparent/children.go), and the config.yaml values the
// registry hands down reach a child as UserSpec.ConfigValues, which a
// template reads as {{ .config.<key> }}.
//
// UserSpec.Variables uses map[string]any because users define arbitrary
// config fields in YAML. A typed struct would require code changes for each
// new variable. Go templates validate variable existence and type at
// render time.
//
// # ChildSpec and hierarchical composition
//
// ChildSpec declares child workers that a parent worker manages:
//
//	children := []config.ChildSpec{
//	    {
//	        Name:       "mqtt-connection",
//	        WorkerType: "mqtt_client",
//	        UserSpec:   config.UserSpec{Config: "url: tcp://..."},
//	        Variables: map[string]any{"URL": "tcp://localhost:1883"},
//	        Enabled: true,
//	    },
//	}
//
// ChildSpec is declarative: parents declare what children should exist, not
// how to create them. The supervisor handles creation, deletion, and updates.
// Children can be added or removed by changing ChildrenSpecs in DeriveDesiredState().
//
// # Child lifecycle gating
//
// A parent gates each child's lifecycle through ChildSpec.Enabled. The
// supervisor's disable-mapping pass translates Enabled into the child's
// Disabled bit, and state files route lifecycle through snap.ShouldStop().
// Setting Enabled=false leaves the child resident in Stopped; flipping it back
// to true resumes the child on the next tick.
//
// # DesiredState and shutdown control
//
// DesiredState includes IsShutdownRequested() for graceful shutdown:
//
//	type DesiredState struct {
//	    State            string
//	    ShutdownRequested bool
//	    ChildrenSpecs    []ChildSpec
//	}
//
// The supervisor sets ShutdownRequested when shutdown is initiated. Workers
// check this flag in their state transitions and complete cleanup states
// before signaling removal. Parent shutdown propagates ShutdownRequested
// to all children, so children clean up before parent completes shutdown.
//
// # Template expansion
//
// A worker renders its Template fields in strict mode (a missing name
// fails): LoadUserSpec decodes UserSpec.Config into the worker's config
// type, BuildNames assembles the names a template runs against, and
// RenderTemplates renders every Template field with those names.
//
// Template expansion happens during DeriveDesiredState(), transforming user
// configuration into concrete desired state.
//
// # Validation
//
// ChildSpec validation checks:
//   - Name is non-empty
//   - WorkerType is registered in factory
//
// See childspec_validation.go for validation details.
package config
