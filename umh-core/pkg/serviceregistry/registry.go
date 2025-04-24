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

// Package portmanager provides functionality to allocate, reserve and manage ports for services

package serviceregistry

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// Registry holds all services required by the application
type Registry struct {
	FileSystem  filesystem.Service
	PortManager portmanager.PortManager
}

// Provider interface defines the methods to access different services
type Provider interface {
	GetFileSystem() filesystem.Service
	GetPortManager() portmanager.PortManager
}

var _ Provider = (*Registry)(nil)

// GetFileSystem returns the filesystem service
func (r *Registry) GetFileSystem() filesystem.Service {
	return r.FileSystem
}

// GetPortManager returns the port manager service
func (r *Registry) GetPortManager() portmanager.PortManager {
	return r.PortManager
}
