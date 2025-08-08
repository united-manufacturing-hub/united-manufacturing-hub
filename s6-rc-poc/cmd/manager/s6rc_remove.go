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
	"os"
	"path/filepath"
)

// Remove stops a service (via exclusion from bundle), removes its definition,
// and applies the changeover.
func (s *S6RCService) Remove(name string) error { // interface method
	if name == "" {
		return errServiceNameRequired
	}

	// Ensure it is not part of the bundle anymore
	if err := s.setBundleMembership(name, false); err != nil {
		// Keep going even if removing membership fails (best-effort)
		s.logWarn("remove: bundle membership update failed", name, err)
	}

	serviceDir := filepath.Join(s.servicesBaseDir, name)
	if err := os.RemoveAll(serviceDir); err != nil {
		return fmt.Errorf("remove service definition %s: %w", serviceDir, err)
	}

	return s.compileAndChangeover()
}
