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
	"regexp"
	"strconv"
)

// AtomicAddDataContractV2 adds a data contract to the dataContractsV2 section,
// starting it at v1.
//
// It writes only that section. Both concepts publish under the address
// _<name>_<version>, so the name is rejected when a data model or a
// dataContracts entry already uses that address.
func (m *FileConfigManager) AtomicAddDataContractV2(ctx context.Context, name string, version DataModelVersion, description string) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	for _, contract := range config.DataContractsV2 {
		if contract.Name == name {
			return fmt.Errorf("another data contract with name %q already exists – choose a unique name", name)
		}
	}

	err = CheckLegacyAddressFree(config, name)
	if err != nil {
		return err
	}

	config.DataContractsV2 = append(config.DataContractsV2, DataContractV2Config{
		Name:        name,
		Description: description,
		Versions: map[string]DataModelVersion{
			"v1": version,
		},
	})

	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicAddDataContractV2(ctx context.Context, name string, version DataModelVersion, description string) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddDataContractV2(ctx, name, version, description)
}

// AtomicAddDataContractV2Version appends the next version to the
// dataContractsV2 entry called name and returns its key, such as "v3".
//
// Versions are append-only: the new key is one above the highest existing
// v<number>, so no existing version is ever changed. The description is left
// as it is.
func (m *FileConfigManager) AtomicAddDataContractV2Version(ctx context.Context, name string, version DataModelVersion) (string, error) {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}

	versionKey, err := addDataContractV2Version(&config, name, version)
	if err != nil {
		return "", err
	}

	err = m.writeConfig(ctx, config)
	if err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	return versionKey, nil
}

func (m *FileConfigManagerWithBackoff) AtomicAddDataContractV2Version(ctx context.Context, name string, version DataModelVersion) (string, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	return m.configManager.AtomicAddDataContractV2Version(ctx, name, version)
}

// addDataContractV2Version appends the next version to the entry called name
// in config and returns its key.
func addDataContractV2Version(config *FullConfig, name string, version DataModelVersion) (string, error) {
	for i, contract := range config.DataContractsV2 {
		if contract.Name != name {
			continue
		}

		versionKey := NextDataContractV2VersionKey(contract.Versions)

		versions := make(map[string]DataModelVersion, len(contract.Versions)+1)
		for key, existing := range contract.Versions {
			versions[key] = existing
		}

		versions[versionKey] = version
		config.DataContractsV2[i].Versions = versions

		return versionKey, nil
	}

	return "", fmt.Errorf("data contract with name %q not found", name)
}

// NextDataContractV2VersionKey returns the key one above the highest
// v<number> in versions, or "v1" when there is none. Keys in another form are
// ignored.
func NextDataContractV2VersionKey(versions map[string]DataModelVersion) string {
	highest := 0

	for key := range versions {
		match := versionKeyPattern.FindStringSubmatch(key)
		if match == nil {
			continue
		}

		number, err := strconv.Atoi(match[1])
		if err == nil && number > highest {
			highest = number
		}
	}

	return "v" + strconv.Itoa(highest+1)
}

var versionKeyPattern = regexp.MustCompile(`^v(\d+)$`)

// CheckLegacyAddressFree returns an error when a data model or a dataContracts
// entry already publishes under _<name>_<version>, the address a
// dataContractsV2 entry called name would take.
func CheckLegacyAddressFree(config FullConfig, name string) error {
	for _, dataModel := range config.DataModels {
		if dataModel.Name == name {
			return fmt.Errorf("a data model named %q already exists and publishes under the same contract address – choose a unique name", name)
		}
	}

	legacyAddress := regexp.MustCompile(`^_` + regexp.QuoteMeta(name) + `_v\d+$`)
	for _, dataContract := range config.DataContracts {
		if legacyAddress.MatchString(dataContract.Name) {
			return fmt.Errorf("the contract address %q is already in use – choose a unique name", dataContract.Name)
		}
	}

	return nil
}

// ContractNameClashes describes every dataContractsV2 entry that shares its
// contract address with a data model or a dataContracts entry, one line per
// entry, in config order. The add actions reject these clashes, but a config
// edited by hand or through set-config-file can still contain them.
func ContractNameClashes(config FullConfig) []string {
	var clashes []string

	for _, contract := range config.DataContractsV2 {
		err := CheckLegacyAddressFree(config, contract.Name)
		if err != nil {
			clashes = append(clashes, fmt.Sprintf("data contract %q: %v", contract.Name, err))
		}
	}

	return clashes
}

// newContractNameClashes returns the clashes in updated that current does not
// already contain, so a config that already holds a clash can still be saved.
func newContractNameClashes(current FullConfig, updated FullConfig) []string {
	existing := make(map[string]bool)
	for _, clash := range ContractNameClashes(current) {
		existing[clash] = true
	}

	var added []string

	for _, clash := range ContractNameClashes(updated) {
		if !existing[clash] {
			added = append(added, clash)
		}
	}

	return added
}

// CheckDataContractV2NameFree returns an error when a dataContractsV2 entry is
// already called name, so a data model called name would share its address.
func CheckDataContractV2NameFree(config FullConfig, name string) error {
	for _, contract := range config.DataContractsV2 {
		if contract.Name == name {
			return fmt.Errorf("a data contract named %q already exists and publishes under the same contract address – choose a unique name", name)
		}
	}

	return nil
}
