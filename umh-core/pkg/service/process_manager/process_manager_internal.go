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

//go:build internal_process_manager

package process_manager

import (
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm"
)

var initOnce sync.Once
var instance Service

// NewDefaultService creates a new default IPM service
func NewDefaultService() Service {
	serviceLogger := logger.For(logger.ComponentS6Service)
	initOnce.Do(func() {
		instance = ipm.NewProcessManager(serviceLogger)
	})
	return instance
}
