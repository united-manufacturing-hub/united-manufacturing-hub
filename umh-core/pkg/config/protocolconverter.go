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

// AtomicAddProtocolConverter adds a protocol converter to the config atomically
func (m *FileConfigManager) AtomicAddProtocolConverter(ctx context.Context, pc ProtocolConverterConfig) error {
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
	for _, cmp := range config.ProtocolConverter {
		if cmp.Name == pc.Name {
			return fmt.Errorf("another protocol converter with name %q already exists – choose a unique name", pc.Name)
		}
	}

	// If it's a child (TemplateRef != Name), verify that a root with that TemplateRef exists
	if pc.ProtocolConverterServiceConfig.TemplateRef != pc.Name {
		templateRef := pc.ProtocolConverterServiceConfig.TemplateRef
		rootExists := false

		// Scan existing protocol converters to find a root with matching name
		for _, existing := range config.ProtocolConverter {
			if existing.Name == templateRef && existing.ProtocolConverterServiceConfig.TemplateRef == existing.Name {
				rootExists = true
				break
			}
		}

		if !rootExists {
			return fmt.Errorf("template %q not found for child %s", templateRef, pc.Name)
		}
	}

	// Add the protocol converter - let convertSpecToYAML handle template generation
	config.ProtocolConverter = append(config.ProtocolConverter, pc)

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicAddProtocolConverter delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicAddProtocolConverter(ctx context.Context, pc ProtocolConverterConfig) error {

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddProtocolConverter(ctx, pc)
}

// AtomicEditProtocolConverter edits a protocol converter in the config atomically
func (m *FileConfigManager) AtomicEditProtocolConverter(ctx context.Context, componentUUID uuid.UUID, pc ProtocolConverterConfig) (ProtocolConverterConfig, error) {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to get config: %w", err)
	}

	// Find target index via GenerateUUIDFromName(Name) == componentUUID
	targetIndex := -1
	var oldConfig ProtocolConverterConfig
	for i, component := range config.ProtocolConverter {
		curComponentID := dataflowcomponentserviceconfig.GenerateUUIDFromName(component.Name)
		if curComponentID == componentUUID {
			targetIndex = i
			oldConfig = config.ProtocolConverter[i]
			break
		}
	}

	if targetIndex == -1 {
		return ProtocolConverterConfig{}, fmt.Errorf("protocol converter with UUID %s not found", componentUUID)
	}

	// Duplicate-name check (exclude the edited one)
	for i, cmp := range config.ProtocolConverter {
		if i != targetIndex && cmp.Name == pc.Name {
			return ProtocolConverterConfig{}, fmt.Errorf("another protocol converter with name %q already exists – choose a unique name", pc.Name)
		}
	}

	newIsRoot := pc.ProtocolConverterServiceConfig.TemplateRef != "" &&
		pc.ProtocolConverterServiceConfig.TemplateRef == pc.Name
	oldIsRoot := oldConfig.ProtocolConverterServiceConfig.TemplateRef != "" &&
		oldConfig.ProtocolConverterServiceConfig.TemplateRef == oldConfig.Name

	// Handle root rename - propagate to children
	if oldIsRoot && newIsRoot && oldConfig.Name != pc.Name {
		// Update all children that reference the old root name
		for i, inst := range config.ProtocolConverter {
			if i != targetIndex && inst.ProtocolConverterServiceConfig.TemplateRef == oldConfig.Name {
				inst.ProtocolConverterServiceConfig.TemplateRef = pc.Name
				config.ProtocolConverter[i] = inst
			}
		}
	}

	// If it's a child (not a root), validate that the template reference exists
	if !newIsRoot {
		templateRef := pc.ProtocolConverterServiceConfig.TemplateRef
		rootExists := false

		// Scan existing protocol converters to find a root with matching name
		// Note: we check the updated slice which may include renamed roots
		for i, inst := range config.ProtocolConverter {
			// Skip the instance being edited since it's not committed yet
			if i == targetIndex {
				continue
			}
			if inst.Name == templateRef && inst.ProtocolConverterServiceConfig.TemplateRef == inst.Name {
				rootExists = true
				break
			}
		}

		// Also check if the new instance itself becomes the root for this template
		if pc.Name == templateRef && newIsRoot {
			rootExists = true
		}

		if !rootExists {
			return ProtocolConverterConfig{}, fmt.Errorf("template %q not found for child %s", templateRef, pc.Name)
		}
	}

	// Commit the edit
	config.ProtocolConverter[targetIndex] = pc

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return ProtocolConverterConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// AtomicDeleteProtocolConverter deletes a protocol converter from the config atomically
func (m *FileConfigManager) AtomicDeleteProtocolConverter(ctx context.Context, componentUUID uuid.UUID) error {
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

	// Find the target protocol converter by UUID
	targetIndex := -1
	var targetConverter ProtocolConverterConfig
	for i, converter := range config.ProtocolConverter {
		converterID := dataflowcomponentserviceconfig.GenerateUUIDFromName(converter.Name)
		if converterID == componentUUID {
			targetIndex = i
			targetConverter = converter
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("protocol converter with UUID %s not found", componentUUID)
	}

	// Determine if target is a root
	isRoot := targetConverter.ProtocolConverterServiceConfig.TemplateRef != "" &&
		targetConverter.ProtocolConverterServiceConfig.TemplateRef == targetConverter.Name

	// If it's a root, check for dependent children
	if isRoot {
		childCount := 0
		for i, converter := range config.ProtocolConverter {
			// Skip the target itself
			if i == targetIndex {
				continue
			}
			// Count children that reference this root
			if converter.ProtocolConverterServiceConfig.TemplateRef == targetConverter.Name {
				childCount++
			}
		}

		if childCount > 0 {
			return fmt.Errorf("cannot delete root %q; %d dependent converters exist", targetConverter.Name, childCount)
		}
	}

	// Build new slice omitting the target
	filteredConverters := make([]ProtocolConverterConfig, 0, len(config.ProtocolConverter)-1)
	for i, converter := range config.ProtocolConverter {
		if i != targetIndex {
			filteredConverters = append(filteredConverters, converter)
		}
	}

	// Update config with filtered converters
	config.ProtocolConverter = filteredConverters

	// write the config
	if err := m.writeConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicEditProtocolConverter delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicEditProtocolConverter(ctx context.Context, componentUUID uuid.UUID, pc ProtocolConverterConfig) (ProtocolConverterConfig, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ProtocolConverterConfig{}, ctx.Err()
	}

	return m.configManager.AtomicEditProtocolConverter(ctx, componentUUID, pc)
}

// AtomicDeleteProtocolConverter delegates to the underlying FileConfigManager
func (m *FileConfigManagerWithBackoff) AtomicDeleteProtocolConverter(ctx context.Context, componentUUID uuid.UUID) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicDeleteProtocolConverter(ctx, componentUUID)
}
