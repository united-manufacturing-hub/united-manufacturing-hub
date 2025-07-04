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

package configmanager

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// AtomicAddDataModel adds a new data model to the config
// the data model is added with the given name and version
// the version is appended to the data model and the config is written back to the file
func (m *FileConfigManager) AtomicAddDataModel(ctx context.Context, name string, dmVersion config.DataModelVersion) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	cfg, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// check for duplicate name before add
	for _, dmc := range cfg.DataModels {
		if dmc.Name == name {
			return fmt.Errorf("another data model with name %q already exists – choose a unique name", name)
		}
	}

	// add the data model to the config
	cfg.DataModels = append(cfg.DataModels, config.DataModelsConfig{
		Name: name,
		Versions: map[string]config.DataModelVersion{
			"v1": dmVersion,
		},
	})

	// write the config back to the file
	err = m.writeConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicAddDataModel(ctx context.Context, name string, dmVersion config.DataModelVersion) error {

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddDataModel(ctx, name, dmVersion)
}

// AtomicEditDataModel edits (append-only) the data model with the given name and appends the new version
// the version is appended to the data model and the config is written back to the file
// we do not allow, editing existing versions, as this would break the data contract
func (m *FileConfigManager) AtomicEditDataModel(ctx context.Context, name string, dmVersion config.DataModelVersion) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	cfg, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	targetIndex := -1
	// find the data model to edit
	for i, dmc := range cfg.DataModels {
		if dmc.Name == name {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("data model with name %q not found", name)
	}

	// get the current data model
	currentDataModel := cfg.DataModels[targetIndex]

	// Find the highest version number to ensure we don't overwrite existing versions
	var maxVersion = 0
	for versionKey := range currentDataModel.Versions {
		if strings.HasPrefix(versionKey, "v") {
			if versionNum, err := strconv.Atoi(versionKey[1:]); err == nil {
				if versionNum > maxVersion {
					maxVersion = versionNum
				}
			}
		}
	}

	// append the new version to the data model
	nextVersion := maxVersion + 1
	currentDataModel.Versions[fmt.Sprintf("v%d", nextVersion)] = dmVersion

	// edit the data model in the config
	cfg.DataModels[targetIndex] = currentDataModel

	// write the config back to the file
	err = m.writeConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicEditDataModel(ctx context.Context, name string, dmVersion config.DataModelVersion) error {

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicEditDataModel(ctx, name, dmVersion)
}

func (m *FileConfigManager) AtomicDeleteDataModel(ctx context.Context, name string) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	cfg, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// find the data model to delete
	targetIndex := -1
	for i, dmc := range cfg.DataModels {
		if dmc.Name == name {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("data model with name %q not found", name)
	}

	// delete the data model from the config
	cfg.DataModels = append(cfg.DataModels[:targetIndex], cfg.DataModels[targetIndex+1:]...)

	// write the config back to the file
	err = m.writeConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicDeleteDataModel(ctx context.Context, name string) error {

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicDeleteDataModel(ctx, name)
}
