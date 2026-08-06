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

// AtomicAddDataModel adds a new data model to the config
// the data model is added with the given name and version
// the version is appended to the data model and the config is written back to the file.
func (m *FileConfigManager) AtomicAddDataModel(ctx context.Context, name string, dmVersion DataModelVersion, description string) error {
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
	for _, dmc := range config.DataModels {
		if dmc.Name == name {
			return fmt.Errorf("another data model with name %q already exists – choose a unique name", name)
		}
	}

	// add the data model to the config
	config.DataModels = append(config.DataModels, DataModelsConfig{
		Name:        name,
		Description: description,
		Versions: map[string]DataModelVersion{
			Version{Major: 1, Minor: 0}.String(): dmVersion,
		},
	})

	// write the config back to the file
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicAddDataModel(ctx context.Context, name string, dmVersion DataModelVersion, description string) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddDataModel(ctx, name, dmVersion, description)
}

// AtomicEditDataModel edits (append-only) the data model with the given name and appends the new version
// the version is appended to the data model and the config is written back to the file
// we do not allow, editing existing versions, as this would break the data contract.
func (m *FileConfigManager) AtomicEditDataModel(ctx context.Context, name string, dmVersion DataModelVersion, description string) error {
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

	targetIndex := -1
	// find the data model to edit
	for i, dmc := range config.DataModels {
		if dmc.Name == name {
			targetIndex = i

			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("data model with name %q not found", name)
	}

	// get the current data model
	currentDataModel := config.DataModels[targetIndex]

	keys := make([]string, 0, len(currentDataModel.Versions))
	for versionKey := range currentDataModel.Versions {
		keys = append(keys, versionKey)
	}

	next, err := NextMinor(keys)
	if err != nil {
		return fmt.Errorf("failed to determine the next version of data model %q: %w", name, err)
	}

	currentDataModel.Versions[next.String()] = dmVersion

	// update the description
	currentDataModel.Description = description

	// edit the data model in the config
	config.DataModels[targetIndex] = currentDataModel

	// write the config back to the file
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicEditDataModel(ctx context.Context, name string, dmVersion DataModelVersion, description string) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicEditDataModel(ctx, name, dmVersion, description)
}

// DataContractNameFor builds the data contract name for a data model version.
// The name always carries the version key, with no exception for minor 0,
// because stream processors rebuild it from the model version and any
// divergence yields a contract that exists nowhere.
func DataContractNameFor(modelName, versionKey string) string {
	return "_" + modelName + "_" + versionKey
}

// AtomicAddDataModelVersionWithContract appends a version to a data model and
// creates the matching data contract in a single config write, then returns the
// version key it wrote. Both land or neither does: the contract is the only way
// to publish a version, and the next-version rule takes the highest minor plus
// one, so a version written without its contract could never be published and
// never be retried into place.
func (m *FileConfigManager) AtomicAddDataModelVersionWithContract(
	ctx context.Context, name string, dmVersion DataModelVersion, description string,
) (string, error) {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}

	targetIndex := -1

	for i, dmc := range config.DataModels {
		if dmc.Name == name {
			targetIndex = i

			break
		}
	}

	if targetIndex == -1 {
		return "", fmt.Errorf("data model with name %q not found", name)
	}

	target := config.DataModels[targetIndex]

	keys := make([]string, 0, len(target.Versions))
	for versionKey := range target.Versions {
		keys = append(keys, versionKey)
	}

	next, err := NextMinor(keys)
	if err != nil {
		return "", fmt.Errorf("failed to determine the next version of data model %q: %w", name, err)
	}

	versionKey := next.String()
	contractName := DataContractNameFor(name, versionKey)

	for _, dcc := range config.DataContracts {
		if dcc.Name == contractName {
			return "", fmt.Errorf("data contract %q already exists, so version %s cannot be added to data model %q", contractName, versionKey, name)
		}
	}

	target.Versions[versionKey] = dmVersion
	target.Description = description
	config.DataModels[targetIndex] = target

	config.DataContracts = append(config.DataContracts, DataContractsConfig{
		Name:  contractName,
		Model: &ModelRef{Name: name, Version: versionKey},
	})

	if err := m.writeConfig(ctx, config); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	return versionKey, nil
}

func (m *FileConfigManagerWithBackoff) AtomicAddDataModelVersionWithContract(
	ctx context.Context, name string, dmVersion DataModelVersion, description string,
) (string, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	return m.configManager.AtomicAddDataModelVersionWithContract(ctx, name, dmVersion, description)
}

func (m *FileConfigManager) AtomicDeleteDataModel(ctx context.Context, name string) error {
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

	// find the data model to delete
	targetIndex := -1

	for i, dmc := range config.DataModels {
		if dmc.Name == name {
			targetIndex = i

			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("data model with name %q not found", name)
	}

	// delete the data model from the config
	config.DataModels = append(config.DataModels[:targetIndex], config.DataModels[targetIndex+1:]...)

	// write the config back to the file
	err = m.writeConfig(ctx, config)
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
