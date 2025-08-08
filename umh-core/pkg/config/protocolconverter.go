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

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

// AtomicAddBridge adds a bridge to the config atomically.
//
// Business logic:
// - Standalone converters: Always allowed if name is unique
// - Root converters (TemplateRef == Name): Always allowed if name is unique, becomes a template for others
// - Child converters (TemplateRef != Name): Only allowed if the referenced template (root) exists
//
// Fails if:
// - Another converter with the same name already exists
// - Adding a child converter but the referenced template doesn't exist
func (m *FileConfigManager) AtomicAddBridge(ctx context.Context, pc BridgeConfig) error {
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
	for _, cmp := range config.Bridge {
		if cmp.Name == pc.Name {
			return fmt.Errorf("another bridge with name %q already exists – choose a unique name", pc.Name)
		}
	}

	// If it's a child (TemplateRef is non-empty and != Name), verify that a root with that TemplateRef exists
	if pc.ServiceConfig.TemplateRef != "" && pc.ServiceConfig.TemplateRef != pc.Name {
		templateRef := pc.ServiceConfig.TemplateRef
		rootExists := false

		// Scan existing bridges to find a root with matching name
		for _, existing := range config.Bridge {
			if existing.Name == templateRef && existing.ServiceConfig.TemplateRef == existing.Name {
				rootExists = true
				break
			}
		}

		if !rootExists {
			return fmt.Errorf("template %q not found for child %s", templateRef, pc.Name)
		}
	}

	// Add the bridge - let convertSpecToYAML handle template generation
	config.Bridge = append(config.Bridge, pc)

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicAddBridge delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicAddBridge(ctx context.Context, pc BridgeConfig) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddBridge(ctx, pc)
}

// AtomicEditBridge edits a bridge in the config atomically.
//
// Business logic:
// - Standalone converters: Can be edited freely (name, config) if new name is unique
// - Root converters: Can be edited, but renaming propagates to all dependent children
// - Child converters: Can be edited, but TemplateRef must point to an existing root
// - Converting between types: Standalone ↔ Root ↔ Child transitions are allowed with validation
//
// Special behaviors:
// - When renaming a root: All children automatically get their TemplateRef updated to the new name
// - When changing a child's TemplateRef: Validates the new template exists
//
// Fails if:
// - Converter with given UUID doesn't exist
// - New name conflicts with another converter (excluding the one being edited)
// - Child converter references a non-existent template
func (m *FileConfigManager) AtomicEditBridge(ctx context.Context, componentUUID uuid.UUID, pc BridgeConfig) (BridgeConfig, error) {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return BridgeConfig{}, fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return BridgeConfig{}, fmt.Errorf("failed to get config: %w", err)
	}

	// Find target index via GenerateUUIDFromName(Name) == componentUUID
	// the index is used later to update the config
	targetIndex := -1
	var oldConfig BridgeConfig
	for i, component := range config.Bridge {
		curComponentID := dataflowcomponentserviceconfig.GenerateUUIDFromName(component.Name)
		if curComponentID == componentUUID {
			targetIndex = i
			oldConfig = config.Bridge[i]
			break
		}
	}

	if targetIndex == -1 {
		return BridgeConfig{}, fmt.Errorf("bridge with UUID %s not found", componentUUID)
	}

	// Duplicate-name check (exclude the edited one)
	for i, cmp := range config.Bridge {
		if i != targetIndex && cmp.Name == pc.Name {
			return BridgeConfig{}, fmt.Errorf("another bridge with name %q already exists – choose a unique name", pc.Name)
		}
	}

	newIsRoot := pc.ServiceConfig.TemplateRef != "" &&
		pc.ServiceConfig.TemplateRef == pc.Name
	oldIsRoot := oldConfig.ServiceConfig.TemplateRef != "" &&
		oldConfig.ServiceConfig.TemplateRef == oldConfig.Name

	// Handle root rename - propagate to children
	if oldIsRoot && newIsRoot && oldConfig.Name != pc.Name {
		// Update all children that reference the old root name
		for i, inst := range config.Bridge {
			if i != targetIndex && inst.ServiceConfig.TemplateRef == oldConfig.Name {
				inst.ServiceConfig.TemplateRef = pc.Name
				config.Bridge[i] = inst
			}
		}
	}

	// If it's a child (TemplateRef is non-empty and not a root), reject the edit
	if !oldIsRoot && pc.ServiceConfig.TemplateRef != "" {
		return BridgeConfig{},
			fmt.Errorf("cannot edit child %q; it is not a root. Edit the root instead: %q",
				oldConfig.Name, oldConfig.ServiceConfig.TemplateRef)
	}

	// Commit the edit
	config.Bridge[targetIndex] = pc

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return BridgeConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// AtomicDeleteBridge deletes a bridge from the config atomically.
//
// Business logic:
// - Standalone bridges: Can always be deleted safely
// - Child bridges: Can always be deleted safely (doesn't affect other converters)
// - Root bridges: Can only be deleted if no children depend on them
//
// Dependency protection:
// - Before deleting a root, checks if any child converters reference it via TemplateRef
// - Deletion is blocked if dependent children exist, preventing orphaned references
// - Error message indicates how many dependent converters exist
//
// Fails if:
// - Converter with given UUID doesn't exist
// - Attempting to delete a root converter that has dependent children
func (m *FileConfigManager) AtomicDeleteBridge(ctx context.Context, componentUUID uuid.UUID) error {
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

	// Find the target bridge by UUID
	targetIndex := -1
	var targetConverter BridgeConfig
	for i, converter := range config.Bridge {
		converterID := dataflowcomponentserviceconfig.GenerateUUIDFromName(converter.Name)
		if converterID == componentUUID {
			targetIndex = i
			targetConverter = converter
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("bridge with UUID %s not found", componentUUID)
	}

	// Determine if target is a root
	isRoot := targetConverter.ServiceConfig.TemplateRef != "" &&
		targetConverter.ServiceConfig.TemplateRef == targetConverter.Name

	// If it's a root, check for dependent children
	if isRoot {
		childCount := 0
		for i, converter := range config.Bridge {
			// Skip the target itself
			if i == targetIndex {
				continue
			}
			// Count children that reference this root
			if converter.ServiceConfig.TemplateRef == targetConverter.Name {
				childCount++
			}
		}

		if childCount > 0 {
			return fmt.Errorf("cannot delete root %q; %d dependent converters exist", targetConverter.Name, childCount)
		}
	}

	// Build new slice omitting the target
	filteredConverters := make([]BridgeConfig, 0, len(config.Bridge)-1)
	for i, converter := range config.Bridge {
		if i != targetIndex {
			filteredConverters = append(filteredConverters, converter)
		}
	}

	// Update config with filtered converters
	config.Bridge = filteredConverters

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicEditBridge delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicEditBridge(ctx context.Context, componentUUID uuid.UUID, pc BridgeConfig) (BridgeConfig, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return BridgeConfig{}, ctx.Err()
	}

	return m.configManager.AtomicEditBridge(ctx, componentUUID, pc)
}

// AtomicDeleteBridge delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicDeleteBridge(ctx context.Context, componentUUID uuid.UUID) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicDeleteBridge(ctx, componentUUID)
}
