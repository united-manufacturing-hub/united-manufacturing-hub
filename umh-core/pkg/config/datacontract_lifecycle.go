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
	"strconv"
	"strings"
)

// ContractAddress is the address a contract version is published under.
//
// The convention is unchanged from before the merge, and is relied on by the
// Console, by benthos and by every existing config, so it is spelled out once here
// rather than composed at each call site.
func ContractAddress(label, version string) string {
	return "_" + label + "_" + version
}

// AtomicAddDataContract creates a contract group at version v1, published under
// _<label>_v1.
//
// One write, not two. Before the merge this took a model write followed by a
// contract write, which left a model with no contract whenever the second failed
// (ENG-5541); there is now nothing to leave half-done.
func (m *FileConfigManager) AtomicAddDataContract(
	ctx context.Context,
	label string,
	structure map[string]Field,
	description string,
) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	if err := refuseIfDegraded(config); err != nil {
		return err
	}

	address := ContractAddress(label, "v1")

	for _, contract := range config.Contracts {
		if contract.Model == label {
			return fmt.Errorf(
				"another data contract with name %q already exists – choose a unique name", label)
		}

		if contract.Name == address {
			return fmt.Errorf("the address %q is already in use – choose a unique name", address)
		}
	}

	config.Contracts = append(config.Contracts, DataContract{
		Name:        address,
		Model:       label,
		Version:     "v1",
		Structure:   structure,
		Description: description,
	})

	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicAddDataContract(
	ctx context.Context,
	label string,
	structure map[string]Field,
	description string,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddDataContract(ctx, label, structure, description)
}

// AtomicAddDataContractVersion appends the next version to an existing group and
// returns the address it was published under.
//
// Versions are append-only: editing a released version would change what an
// address already in use enforces, which is the one thing a contract exists to
// prevent.
func (m *FileConfigManager) AtomicAddDataContractVersion(
	ctx context.Context,
	label string,
	structure map[string]Field,
	description string,
) (string, string, error) {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return "", "", fmt.Errorf("failed to get config: %w", err)
	}

	if err := refuseIfDegraded(config); err != nil {
		return "", "", err
	}

	if !hasGroup(config.Contracts, label) {
		return "", "", fmt.Errorf("data model with name %q not found", label)
	}

	version := nextVersionKey(config.Contracts, label)
	address := ContractAddress(label, version)

	for _, contract := range config.Contracts {
		if contract.Name == address {
			return "", "", fmt.Errorf("the address %q is already in use", address)
		}
	}

	// The description belongs to the group, so a new version restates it for every
	// entry in the group. Leaving the old entries alone would make the emitted YAML
	// depend on which entry happened to be written first.
	for i := range config.Contracts {
		if config.Contracts[i].Model == label {
			config.Contracts[i].Description = description
		}
	}

	config.Contracts = append(config.Contracts, DataContract{
		Name:        address,
		Model:       label,
		Version:     version,
		Structure:   structure,
		Description: description,
	})

	if err := m.writeConfig(ctx, config); err != nil {
		return "", "", fmt.Errorf("failed to write config: %w", err)
	}

	return address, version, nil
}

func (m *FileConfigManagerWithBackoff) AtomicAddDataContractVersion(
	ctx context.Context,
	label string,
	structure map[string]Field,
	description string,
) (string, string, error) {
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}

	return m.configManager.AtomicAddDataContractVersion(ctx, label, structure, description)
}

// AtomicDeleteDataContract removes a single address, leaving its structure in
// place.
//
// The version survives as a definition, so anything referencing it through
// _refModel keeps resolving and its registry subjects keep validating. This is also
// how a bare address left over from before the merge gets cleaned up.
func (m *FileConfigManager) AtomicDeleteDataContract(ctx context.Context, name string) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	if err := refuseIfDegraded(config); err != nil {
		return err
	}

	target := -1

	for i, contract := range config.Contracts {
		if contract.Name == name {
			target = i

			break
		}
	}

	if target == -1 {
		return fmt.Errorf("data contract with name %q not found", name)
	}

	if refs := FindAddressReferences(config, name); len(refs) > 0 {
		return fmt.Errorf("cannot delete data contract %q: still referenced by %s",
			name, describeReferences(refs))
	}

	if config.Contracts[target].Model == "" {
		// A bare address carries no structure, so there is nothing to keep.
		config.Contracts = append(config.Contracts[:target], config.Contracts[target+1:]...)
	} else {
		config.Contracts[target].Name = ""
		config.Contracts[target].DefaultBridges = nil
	}

	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicDeleteDataContract(ctx context.Context, name string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicDeleteDataContract(ctx, name)
}

// AtomicDeleteDataContractGroup removes a whole group: every version, every
// address, and the structures.
//
// It refuses while anything still references the group. Before the merge, deleting
// a data model left its contract pointing at nothing, which silently stopped
// validating a topic that was still being published to -- so the refusal is the
// reason this operation is safe to expose at all, not a nicety on top of it.
func (m *FileConfigManager) AtomicDeleteDataContractGroup(ctx context.Context, label string) error {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	if err := refuseIfDegraded(config); err != nil {
		return err
	}

	if !hasGroup(config.Contracts, label) {
		return fmt.Errorf("data model with name %q not found", label)
	}

	if refs := FindModelReferences(config, label); len(refs) > 0 {
		return fmt.Errorf("cannot delete data contract %q: still referenced by %s",
			label, describeReferences(refs))
	}

	kept := make([]DataContract, 0, len(config.Contracts))

	for _, contract := range config.Contracts {
		if contract.Model != label {
			kept = append(kept, contract)
		}
	}

	config.Contracts = kept

	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m *FileConfigManagerWithBackoff) AtomicDeleteDataContractGroup(ctx context.Context, label string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicDeleteDataContractGroup(ctx, label)
}

// refuseIfDegraded blocks mutation of a config whose conversion could not be
// verified.
//
// Writing would either persist the change into sections we are no longer deriving
// from -- losing it on the next read -- or replace the file with a shape we cannot
// vouch for. Refusing is the only option that keeps the user's file intact.
func refuseIfDegraded(config FullConfig) error {
	if config.ContractsDegraded {
		return errContractsNotLossless
	}

	return nil
}

func hasGroup(contracts []DataContract, label string) bool {
	for _, contract := range contracts {
		if contract.Model == label {
			return true
		}
	}

	return false
}

// nextVersionKey mints the version after the highest existing one.
//
// This is the pre-merge loop unchanged, including that it ignores keys it cannot
// parse: a version named "draft" does not participate in the ordering rather than
// blocking the operation. Changing that belongs to the minor-version work, not
// here.
func nextVersionKey(contracts []DataContract, label string) string {
	maxVersion := 0

	for _, contract := range contracts {
		if contract.Model != label {
			continue
		}

		if strings.HasPrefix(contract.Version, "v") {
			if versionNum, err := strconv.Atoi(contract.Version[1:]); err == nil {
				if versionNum > maxVersion {
					maxVersion = versionNum
				}
			}
		}
	}

	return fmt.Sprintf("v%d", maxVersion+1)
}
