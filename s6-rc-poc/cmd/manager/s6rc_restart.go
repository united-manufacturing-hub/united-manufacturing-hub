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
)

// Restart performs an in-place restart using s6-rc without a compile.
func (s *S6RCService) Restart(name string) error { // interface method
	if name == "" {
		return errServiceNameRequired
	}

	// Block on locks to avoid transient failures when another s6-rc is active
	err := s.run("s6-rc", "-b", "-r", "change", name)
	if err != nil {
		return fmt.Errorf("restart %s: %w", name, err)
	}

	return nil
}
