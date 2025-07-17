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

// Package serviceregistry provides a centralized registry for accessing core services like portmanager and filesystem

package serviceregistry

import (
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var (
	globalRegistry *Registry
	initOnce       sync.Once
)

func NewRegistry() *Registry {
	initOnce.Do(func() {
		pm := portmanager.NewDefaultPortManager()
		fs := filesystem.NewDefaultService()
		globalRegistry = &Registry{
			PortManager: pm,
			FileSystem:  fs,
		}
	})
	return globalRegistry
}

// GetGlobalRegistry returns the global registry instance.
// This function is used to be called inside the manager.CreateSnapshot which might not have the service registry dependency injected.
func GetGlobalRegistry() *Registry {
	return NewRegistry()
}
