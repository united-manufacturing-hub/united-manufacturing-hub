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
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// ExitHistory prints (logs) the recent event/exit history using s6-svdt.
func (s *S6RCService) ExitHistory(name string) error { // interface method
	if name == "" {
		return errServiceNameRequired
	}

	serviceRunDir := filepath.Join("/run/service", name)
	stdout, err := s.runCapture("s6-svdt", serviceRunDir)

	if err != nil {
		return fmt.Errorf("exit history %s: %w", name, err)
	}

	if s.logger != nil {
		s.logger.Info("Service exit history", zap.String("service", name), zap.String("history", strings.TrimSpace(stdout)))
	}

	return nil
}
