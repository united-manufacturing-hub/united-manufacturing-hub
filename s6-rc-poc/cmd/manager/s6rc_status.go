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

package manager

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// Status prints (logs) the current status of the service via s6-svstat.
func (s *S6RCService) Status(name string) error { //nolint:ireturn // interface method
	if name == "" {
		return errors.New("service name is required")
	}
	serviceRunDir := filepath.Join("/run/service", name)
	stdout, _, err := s.run("s6-svstat", serviceRunDir)
	if err != nil {
		return fmt.Errorf("status %s: %w", name, err)
	}
	if s.logger != nil {
		s.logger.Info("Service status", zap.String("service", name), zap.String("status", strings.TrimSpace(stdout)))
	}
	return nil
}
