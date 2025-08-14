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

// AtomicAddStreamProcessor adds a stream processor to the config atomically.
//
// Business logic:
// - Standalone processors: Always allowed if name is unique
// - Root processors (TemplateRef == Name): Always allowed if name is unique, becomes a template for others
// - Child processors (TemplateRef != Name): Only allowed if the referenced template (root) exists
//
// Fails if:
// - Another processor with the same name already exists
// - Adding a child processor but the referenced template doesn't exist.
func (m *FileConfigManager) AtomicAddStreamProcessor(ctx context.Context, streamProcessorConfig StreamProcessorConfig) error {
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
	for _, cmp := range config.StreamProcessor {
		if cmp.Name == streamProcessorConfig.Name {
			return fmt.Errorf("another stream processor with name %q already exists – choose a unique name", streamProcessorConfig.Name)
		}
	}

	// If it's a child (TemplateRef is non-empty and != Name), verify that a root with that TemplateRef exists
	if streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef != "" && streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef != streamProcessorConfig.Name {
		templateRef := streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef
		rootExists := false

		// Scan existing stream processors to find a root with matching name
		for _, existing := range config.StreamProcessor {
			if existing.Name == templateRef && existing.StreamProcessorServiceConfig.TemplateRef == existing.Name {
				rootExists = true

				break
			}
		}

		if !rootExists {
			return fmt.Errorf("template %q not found for child %s", templateRef, streamProcessorConfig.Name)
		}
	}

	// Add the stream processor - let convertSpecToYAML handle template generation
	config.StreamProcessor = append(config.StreamProcessor, streamProcessorConfig)

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicAddStreamProcessor delegates to the underlying FileConfigManager.
func (m *FileConfigManagerWithBackoff) AtomicAddStreamProcessor(ctx context.Context, streamProcessorConfig StreamProcessorConfig) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicAddStreamProcessor(ctx, streamProcessorConfig)
}

// AtomicEditStreamProcessor edits a stream processor in the config atomically.
//
// Business logic:
// - Can edit standalone processors freely
// - Can edit root processors (the changes affect all children that inherit from it)
// - Cannot edit child processors directly - must edit the root template instead
//
// Fails if:
// - The processor with the given name doesn't exist
// - Attempting to edit a child processor (TemplateRef != "" && TemplateRef != Name)
// - The new configuration has validation errors.
func (m *FileConfigManager) AtomicEditStreamProcessor(ctx context.Context, streamProcessorConfig StreamProcessorConfig) (StreamProcessorConfig, error) {
	err := m.mutexAtomicUpdate.Lock(ctx)
	if err != nil {
		return StreamProcessorConfig{}, fmt.Errorf("failed to lock config file: %w", err)
	}
	defer m.mutexAtomicUpdate.Unlock()

	// get the current config
	config, err := m.GetConfig(ctx, 0)
	if err != nil {
		return StreamProcessorConfig{}, fmt.Errorf("failed to get config: %w", err)
	}

	// find the stream processor by name
	var (
		oldStreamProcessor *StreamProcessorConfig
		index              = -1
	)

	for i, cmp := range config.StreamProcessor {
		if cmp.Name == streamProcessorConfig.Name {
			oldStreamProcessor = &cmp
			index = i

			break
		}
	}

	if oldStreamProcessor == nil {
		return StreamProcessorConfig{}, fmt.Errorf("stream processor with name %q not found", streamProcessorConfig.Name)
	}

	// Check if the old stream processor is a root (TemplateRef == Name or TemplateRef == "")
	oldIsRoot := oldStreamProcessor.StreamProcessorServiceConfig.TemplateRef == "" || oldStreamProcessor.StreamProcessorServiceConfig.TemplateRef == oldStreamProcessor.Name

	// Prevent editing child stream processors directly
	if !oldIsRoot && oldStreamProcessor.StreamProcessorServiceConfig.TemplateRef != "" {
		return StreamProcessorConfig{}, fmt.Errorf("cannot edit child stream processor %q directly – edit the root template %q instead", streamProcessorConfig.Name, oldStreamProcessor.StreamProcessorServiceConfig.TemplateRef)
	}

	// Store the old config for return
	oldConfig := *oldStreamProcessor

	// Update the stream processor
	config.StreamProcessor[index] = streamProcessorConfig

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return StreamProcessorConfig{}, fmt.Errorf("failed to write config: %w", err)
	}

	return oldConfig, nil
}

// AtomicEditStreamProcessor delegates to the underlying FileConfigManager.
func (m *FileConfigManagerWithBackoff) AtomicEditStreamProcessor(ctx context.Context, streamProcessorConfig StreamProcessorConfig) (StreamProcessorConfig, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return StreamProcessorConfig{}, ctx.Err()
	}

	return m.configManager.AtomicEditStreamProcessor(ctx, streamProcessorConfig)
}

// AtomicDeleteStreamProcessor deletes a stream processor from the config atomically.
//
// Business logic:
// - Can delete standalone processors freely
// - Can delete root processors ONLY if no children depend on them
// - Can delete child processors freely (they just stop inheriting from their root)
//
// Fails if:
// - The processor with the given name doesn't exist
// - Attempting to delete a root processor that still has children depending on it.
func (m *FileConfigManager) AtomicDeleteStreamProcessor(ctx context.Context, name string) error {
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

	// find the stream processor by name
	var (
		streamProcessorToDelete *StreamProcessorConfig
		index                   = -1
	)

	for i, cmp := range config.StreamProcessor {
		if cmp.Name == name {
			streamProcessorToDelete = &cmp
			index = i

			break
		}
	}

	if streamProcessorToDelete == nil {
		return fmt.Errorf("stream processor with name %q not found", name)
	}

	// Check if this is a root processor (TemplateRef == Name)
	isRoot := streamProcessorToDelete.StreamProcessorServiceConfig.TemplateRef == streamProcessorToDelete.Name

	// If it's a root, check if any children depend on it
	if isRoot {
		for _, cmp := range config.StreamProcessor {
			// Skip the processor we're trying to delete
			if cmp.Name == name {
				continue
			}
			// Check if this processor is a child of the one we're trying to delete
			if cmp.StreamProcessorServiceConfig.TemplateRef == name {
				return fmt.Errorf("cannot delete root stream processor %q because child %q depends on it", name, cmp.Name)
			}
		}
	}

	// Remove the stream processor
	config.StreamProcessor = append(config.StreamProcessor[:index], config.StreamProcessor[index+1:]...)

	// write the config
	err = m.writeConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AtomicDeleteStreamProcessor delegates to the underlying FileConfigManager.
func (m *FileConfigManagerWithBackoff) AtomicDeleteStreamProcessor(ctx context.Context, name string) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return m.configManager.AtomicDeleteStreamProcessor(ctx, name)
}
