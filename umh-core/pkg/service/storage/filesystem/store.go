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

// Package filesystem implements the storage interface for the raw file storage
package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/storage"
	"gopkg.in/yaml.v3"
)

// Config contains the configuration for the filesystem storage
type Config struct {
	BasePath         string
	TempPath         string
	CleanupTempFiles bool
}

var _ storage.KVInterface = &Store{}

// Store implements the storage interface for the filesystem storage
type Store struct {
	// mu is the mutex to protect the filesystem operations
	mu sync.RWMutex
	// config is the filesystem storage configuration
	config Config
	// versioner handles the verisoning of the objects for file storage
	versioner storage.Versioner
	// serializer encodes and decodes the objects for file storage
	serializer storage.Serializer
	// For clean shutdown
	stopCh chan struct{}
}

// NewStore creates a new filesystem storage
func NewStore(config Config) (*Store, error) {
	if config.BasePath == "" {
		return nil, errors.New("config base path cannot be empty")
	}

	if err := os.MkdirAll(config.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base path for file storage %s: %w", config.BasePath, err)
	}

	if config.TempPath == "" {
		config.TempPath = filepath.Join(config.BasePath, ".tmp")
	}

	if err := os.MkdirAll(config.TempPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp path for file storage %s: %w", config.TempPath, err)
	}

	s := &Store{
		config:     config,
		versioner:  storage.NewSimpleVersioner(),
		serializer: storage.NewYamlSerializer(),
		stopCh:     make(chan struct{}),
	}
	return s, nil
}

// keyToPath converts the key to a file path
func (s *Store) keyToPath(key string) string {
	//Sanitize the key
	key = strings.Replace(key, "//", "/", -1)
	key = strings.Replace(key, "../", "", -1)
	return filepath.Join(s.config.BasePath, key)
}

func (s *Store) Versioner() storage.Versioner {
	return s.versioner
}

// Create creates a new object in the filesystem storage
func (s *Store) Create(ctx context.Context, key string, object storage.Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.keyToPath(key)

	if _, err := os.Stat(filePath); err == nil {
		return storage.ErrKeyExists
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for file storage %s: %w", filePath, err)
	}

	if err := s.versioner.UpdateVersion(object); err != nil {
		return fmt.Errorf("failed to update version for file storage object %s: %w", filePath, err)
	}

	// Serialize the object
	data, err := s.serializer.Encode(object)
	if err != nil {
		return fmt.Errorf("failed to serialize object for file storage %s: %w", filePath, err)
	}

	// Atomically write this object
	// First write to a temp file and then move it to the final destination
	// Why atomically?
	// - Crash safety: If umh-core container crashes during the write, the original config object is not corrupted
	// - Atomicity: The file is either fully written or not at all. os.Rename is atomic in most filesystem
	// - Consistency: Readers of the object will never see a partial write. Otherwise we need to use costly sync.Mutex to lock the file and maintain consistency between read and write
	tempFile, err := os.CreateTemp(s.config.TempPath, "create")
	if err != nil {
		return fmt.Errorf("failed to create temp file for file storage %s: %w", filePath, err)
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("failed to write object to temp file for file storage %s: %w", filePath, err)
	}

	// Flush the contents
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file for file storage %s: %w", filePath, err)
	}

	// Ensure the temp file is closed before renaming.
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file for file storage %s: %w", filePath, err)
	}

	if err := os.Rename(tempFile.Name(), filePath); err != nil {
		return fmt.Errorf("failed to rename temp file for file storage %s: %w", filePath, err)
	}

	return nil
}

// Delete deletes an object from the filesystem storage
func (s *Store) Delete(ctx context.Context, key string) error {
	return nil
}

// Get returns an object from the filesystem storage
func (s *Store) Get(ctx context.Context, key string, ignoreNotFound bool) (storage.Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.keyToPath(key)

	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		if ignoreNotFound {
			return nil, nil
		}
		return nil, storage.ErrKeyNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read file for file storage %s: %w", filePath, err)
	}

	var objMap map[string]interface{}
	if err := yaml.Unmarshal(data, &objMap); err != nil {
		return nil, fmt.Errorf("failed to decode object for file storage %s: %w", filePath, err)
	}

	obj := &dynamicObject{
		data: objMap,
	}

	return obj, nil

}

// GuaranteedUpdate updates the object in the storage layer for the given key
// and keeps retrying the updated until success
// TODO: Introduce max retries and backoff
func (s *Store) GuaranteedUpdate(ctx context.Context, key string, ignoreNotFound bool, precondition func(existingObject storage.Object) error, updateFunc func(existingObject storage.Object) (storage.Object, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.keyToPath(key)

	existingObject, err := s.Get(ctx, key, ignoreNotFound)
	if err != nil {
		return fmt.Errorf("failed to get object for file storage %s: %w", filePath, err)
	}

	if err := precondition(existingObject); err != nil {
		return fmt.Errorf("failed to check precondition for file storage %s: %w", filePath, err)
	}

	for {
		updatedObject, err := updateFunc(existingObject)
		if err != nil {
			return fmt.Errorf("failed to update object for file storage %s: %w", filePath, err)
		}

		if err := s.versioner.UpdateVersion(updatedObject); err != nil {
			return fmt.Errorf("failed to update version for file storage object %s: %w", filePath, err)
		}

		// Serialize the object
		data, err := s.serializer.Encode(updatedObject)
		if err != nil {
			return fmt.Errorf("failed to serialize object for file storage %s: %w", filePath, err)
		}

		// Atomically update the object by writing to a temp file and then moving it to the final destination
		tempFile, err := os.CreateTemp(s.config.TempPath, "update")
		if err != nil {
			return fmt.Errorf("failed to create temp file for file storage %s: %w", filePath, err)
		}
		defer tempFile.Close()
		defer os.Remove(tempFile.Name())

		if _, err := tempFile.Write(data); err != nil {
			return fmt.Errorf("failed to write object to temp file for file storage %s: %w", filePath, err)
		}

		// Flush the contents
		if err := tempFile.Sync(); err != nil {
			return fmt.Errorf("failed to sync temp file for file storage %s: %w", filePath, err)
		}

		// Ensure the temp file is closed before renaming.
		if err := tempFile.Close(); err != nil {
			return fmt.Errorf("failed to close temp file for file storage %s: %w", filePath, err)
		}

		if err := os.Rename(tempFile.Name(), filePath); err != nil {
			// If rename fails, retry the whole operation
			continue
		}

		return nil
	}
}

// Update implements storage.Interface.
func (s *Store) Update(ctx context.Context, key string, obj storage.Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.keyToPath(key)

	if _, err := os.Stat(filePath); err != nil {
		return storage.ErrKeyNotFound
	}

	// Atomically update the object by writing to a temp file and then moving it to the final destination
	tempFile, err := os.CreateTemp(s.config.TempPath, "update")
	if err != nil {
		return fmt.Errorf("failed to create temp file for file storage %s: %w", filePath, err)
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	if err := s.versioner.UpdateVersion(obj); err != nil {
		return fmt.Errorf("failed to update version for file storage object %s: %w", filePath, err)
	}

	// Serialize the object
	data, err := s.serializer.Encode(obj)
	if err != nil {
		return fmt.Errorf("failed to serialize object for file storage %s: %w", filePath, err)
	}

	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("failed to write object to temp file for file storage %s: %w", filePath, err)
	}

	// Flush the contents
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file for file storage %s: %w", filePath, err)
	}

	// Ensure the temp file is closed before renaming.
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file for file storage %s: %w", filePath, err)
	}

	if err := os.Rename(tempFile.Name(), filePath); err != nil {
		return fmt.Errorf("failed to rename temp file for file storage %s: %w", filePath, err)
	}

	return nil
}

// dynamicObject is a simple implementation of storage.Object
type dynamicObject struct {
	data map[string]interface{}
}

func (d *dynamicObject) GetResourceVersion() string {
	if rv, ok := d.data["resourceVersion"].(string); ok {
		return rv
	}
	return ""
}

func (d *dynamicObject) SetResourceVersion(version string) {
	d.data["resourceVersion"] = version
}
