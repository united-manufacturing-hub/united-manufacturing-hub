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

package config

import (
	"context"
	"fmt"
)

// AtomicAddDataContract adds a new data contract to the config
// Data contracts can only be added, never edited or deleted
func (m *FileConfigManager) AtomicAddDataContract(ctx context.Context, dataContract DataContractsConfig) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// check for duplicate name before add
	for _, dcc := range config.DataContracts {
		if dcc.Name == dataContract.Name {
			return fmt.Errorf("another data contract with name %q already exists – choose a unique name", dataContract.Name)
		}
	}

	// add the data contract to the config
	config.DataContracts = append(config.DataContracts, dataContract)

	// write the config back to the file
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicAddDataContract(ctx context.Context, dataContract DataContractsConfig) error {

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddDataContract(ctx, dataContract)
}
