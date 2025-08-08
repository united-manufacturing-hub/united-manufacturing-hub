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

// Package configwatcher provides an interface that upon receiving a path to a config, emits events when it changes.
// These can be:
// DELETE (name)
// CREATE (name, service details, desired state)
// CONFIG_CHANGE (name, service details)
// STATE_CHANGE (name, desired state)
package configwatcher

// ConfigWatcher is an interface that provides a way to watch for changes to a config file.
// It is used to notify the s6-rc service when the config file changes.
// The service will then reload the config file and update the s6-rc service.
type ConfigWatcher interface {
	Start(configPath string) error
	Stop() error
	Events() chan ConfigEvent
}
