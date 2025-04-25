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
	"fmt"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var (
	instance Registry
	once     sync.Once
)

func NewRegistry() (*Registry, error) {

	//  Initialize the registry only once
	// Since the registry contains services like portmanager which should be initialized only once and shared between services
	// we need to make sure that the registry is initialized only once
	var err error
	once.Do(func() {
		minPort := 9000
		maxPort := 9999
		pm, portErr := portmanager.NewDefaultPortManager(minPort, maxPort)
		if portErr != nil {
			err = fmt.Errorf("failed to create port manager: %w", portErr)
		}
		fs := filesystem.NewDefaultService()
		instance = Registry{
			PortManager: pm,
			FileSystem:  fs,
		}
	})
	return &instance, err
}
