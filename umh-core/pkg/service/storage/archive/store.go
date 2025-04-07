/*
 *   Copyright (c) 2025 UMH Systems GmbH
 *   All rights reserved.

 *   Licensed under the Apache License, Version 2.0 (the "License");
 *   you may not use this file except in compliance with the License.
 *   You may obtain a copy of the License at

 *   http://www.apache.org/licenses/LICENSE-2.0

 *   Unless required by applicable law or agreed to in writing, software
 *   distributed under the License is distributed on an "AS IS" BASIS,
 *   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *   See the License for the specific language governing permissions and
 *   limitations under the License.
 */

// Package archive implements the storage interface for the archive storage
package archive

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/storage"
)

var (
	ErrVersionNotFound = errors.New("version not found")
)

// VersionMetadata tracks the metadata for the specific version of an resource object
// Example of resource object: config.yaml
type VersionMetadata struct {
	Version     string    `json:"version"`
	Timestamp   time.Time `json:"timestamp"`
	Editor      string    `json:"editor"`
	ChangeType  string    `json:"changeType"` // create, update, delete
	Description string    `json:"description"`
}

// ArchiveConfigStore is the storage layer for the archive configuration
type ArchiveConfigStore struct {
	store          storage.KVInterface
	versionsPrefix string
	dfcPrefix      string
	locationPrefix string
}

func NewArchiveConfigStore(store storage.KVInterface, versionsPrefix, dfcPrefix, locationPrefix string) *ArchiveConfigStore {
	if versionsPrefix == "" {
		versionsPrefix = "versions"
	}
	if dfcPrefix == "" {
		dfcPrefix = "dfc_versions/"
	}
	if locationPrefix == "" {
		locationPrefix = "locations/"
	}

	return &ArchiveConfigStore{
		store:          store,
		versionsPrefix: versionsPrefix,
		dfcPrefix:      dfcPrefix,
		locationPrefix: locationPrefix,
	}
}

// VersionKey returns the key for the specified version
func (acs *ArchiveConfigStore) VersionKey(configKey string, version string) string {
	return filepath.Join(acs.versionsPrefix, configKey, version)
}

// DFCVersionKey returns the key for the specified DFC version
func (acs *ArchiveConfigStore) DFCVersionKey(configKey string, version string) string {
	return filepath.Join(acs.dfcPrefix, configKey, version)
}

// LocationKey returns the key for the specified location
func (acs *ArchiveConfigStore) LocationKey(configKey string, location string) string {
	return filepath.Join(acs.locationPrefix, configKey, location)
}

// StoreVersion stores the new version of the resource object
// Should be called when a new version of config.yaml should be updated
func (acs *ArchiveConfigStore) StoreVersion(
	ctx context.Context,
	configKey string,
	object storage.Object,
	metadata VersionMetadata,
) error {

	if metadata.Version == "" {
		metadata.Version = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	// Set timestamp if not provided
	if metadata.Timestamp.IsZero() {
		metadata.Timestamp = time.Now()
	}

	// Set resource version on the object
	object.SetResourceVersion(metadata.Version)

	// Store the object in the versions path
	versionKey := acs.VersionKey(configKey, metadata.Version)
	if err := acs.store.Create(ctx, versionKey, object); err != nil {
		return fmt.Errorf("failed to store version: %w", err)
	}

	// Store the metadata as a separate object
	metadataObj := &dynamicObject{
		data: map[string]interface{}{
			"version":     metadata.Version,
			"timestamp":   metadata.Timestamp,
			"editor":      metadata.Editor,
			"changeType":  metadata.ChangeType,
			"description": metadata.Description,
		},
	}

	metadataKey := acs.VersionKey(configKey, metadata.Version+".metadata")
	if err := acs.store.Create(ctx, metadataKey, metadataObj); err != nil {
		return fmt.Errorf("failed to store version metadata: %w", err)
	}

	// If this is a DFC operation, record in DFC versions
	if metadata.ChangeType != "" {
		dfcObj := &dynamicObject{
			data: map[string]interface{}{
				"version":    metadata.Version,
				"changeType": metadata.ChangeType,
			},
		}

		dfcKey := acs.DFCVersionKey(configKey, metadata.Version)
		if err := acs.store.Create(ctx, dfcKey, dfcObj); err != nil {
			return fmt.Errorf("failed to store DFC version: %w", err)
		}
	}

	// For created or modified changes, update the location versions
	if metadata.ChangeType == "created" || metadata.ChangeType == "modified" {
		locObj := &dynamicObject{
			data: map[string]interface{}{
				"version": metadata.Version,
			},
		}
		locKey := acs.LocationKey(configKey, metadata.Version)
		if err := acs.store.Create(ctx, locKey, locObj); err != nil {
			return fmt.Errorf("failed to store location version: %w", err)
		}
	}
	return nil
}

type dynamicObject struct {
	data map[string]interface{}
}

func (d *dynamicObject) GetResourceVersion() string {
	return d.data["version"].(string)
}

func (d *dynamicObject) SetResourceVersion(version string) {
	d.data["version"] = version
}
