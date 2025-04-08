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

package redpanda_monitor

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
)

// Ensure RedpandaMockMonitorService implements IRedpandaMonitorService
var _ IRedpandaMonitorService = (*RedpandaMockMonitorService)(nil)

type RedpandaMockMonitorService struct {
	logger       *zap.Logger
	metricsState *RedpandaMetricsState
}

func NewMockRedpandaMonitorService() *RedpandaMockMonitorService {
	log := logger.New(logger.ComponentRedpandaMonitorService, logger.FormatJSON)
	return &RedpandaMockMonitorService{
		logger:       log,
		metricsState: NewRedpandaMetricsState(),
	}
}

func (s *RedpandaMockMonitorService) GenerateS6ConfigForRedpandaMonitor() (s6serviceconfig.S6ServiceConfig, error) {

	s6Config := s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/bin/sh",
			fmt.Sprintf("%s/%s/config/run_redpanda_monitor.sh", constants.S6BaseDir, "redpanda-monitor-mock"),
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			"run_redpanda_monitor.sh": "mocked script content",
		},
	}

	return s6Config, nil
}

func (s *RedpandaMockMonitorService) Status(ctx context.Context, filesystemService filesystem.Service, tick uint64) (ServiceInfo, error) {
	return ServiceInfo{}, nil
}

func (s *RedpandaMockMonitorService) AddRedpandaToS6Manager(ctx context.Context) error {
	return nil
}

func (s *RedpandaMockMonitorService) RemoveRedpandaFromS6Manager(ctx context.Context) error {
	return nil
}

func (s *RedpandaMockMonitorService) StartRedpanda(ctx context.Context) error {
	return nil
}

func (s *RedpandaMockMonitorService) StopRedpanda(ctx context.Context) error {
	return nil
}

func (s *RedpandaMockMonitorService) ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (error, bool) {
	return nil, false
}

func (s *RedpandaMockMonitorService) ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool {
	return false
}
