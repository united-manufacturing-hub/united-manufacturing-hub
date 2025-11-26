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

package example_panic

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"go.uber.org/zap"
)

type Connection interface{}

type ConnectionPool interface {
	Acquire() (Connection, error)
	Release(Connection) error
	HealthCheck(Connection) error
}

type DefaultConnectionPool struct{}

func (d *DefaultConnectionPool) Acquire() (Connection, error) {
	return nil, nil
}

func (d *DefaultConnectionPool) Release(_ Connection) error {
	return nil
}

func (d *DefaultConnectionPool) HealthCheck(_ Connection) error {
	return nil
}

type PanicDependencies struct {
	*fsmv2.BaseDependencies
	connectionPool ConnectionPool
}

func NewPanicDependencies(connectionPool ConnectionPool, logger *zap.SugaredLogger, workerType, workerID string) *PanicDependencies {
	return &PanicDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger, workerType, workerID),
		connectionPool:   connectionPool,
	}
}

func (d *PanicDependencies) GetConnectionPool() ConnectionPool {
	return d.connectionPool
}
