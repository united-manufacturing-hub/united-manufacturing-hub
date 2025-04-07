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

// Package storage provides a common interface for all storage services
// Specially designed for the storage operations of the config files in the key-value store
// This package only focuses on storing the objects and reading them back from the storage layer into the objects
package storage

import "context"

// Object represents the object that should be versioned
// For example, a config file config.yaml is an object
// Every struct has to implement this interface to be able to store and retrieve from the storage layer
// Why do we need this interface ?
// We need this interface so that the storage package doesn't need to know the concrete type of the object being stored
type Object interface {
	GetResourceVersion() string
	SetResourceVersion(resourceVersion string)
}

// Versioner abstract the versioning of the storage files
type Versioner interface {
	// UpdateVersion updates the version of the object
	UpdateVersion(object Object) error
	// CompareResourceVersions compares resource versions of two objects
	// Returns -1 if lhs is less than rhs, 0 if they are equal, and 1 if lhs is greater than rhs
	CompareResourceVersions(lhs, rhs string) int
	// ParseResourceVersion parses a string resource version to uint64
	ParseResourceVersion(resourceVersion string) (uint64, error)
}

// StorageEventType represents the type of the storage event
// This event types will be used by the archive storage to determine the type of the event
type StorageEventType string

const (
	Created StorageEventType = "CREATED"
	Updated StorageEventType = "UPDATED"
	Deleted StorageEventType = "DELETED"
)

// Interface offers a common interface for data marshalling and unmarshalling
// and hides all the underlying storage related operations
type Interface interface {
	//Versioner returns versioner for the storage type
	Versioner() Versioner

	Create(ctx context.Context, key string, object Object) error
	// Get retrieves the object from the storage layer for the given key
	Get(ctx context.Context, key string, ignoreNotFound bool) (Object, error)
	// Update updates the object in the storage layer for the given key
	Update(ctx context.Context, key string, obj Object) error
	// GuaranteedUpdate updates the object in the storage layer for the given key
	// and keeps retrying the updated until success
	GuaranteedUpdate(ctx context.Context, key string, ignoreNotFound bool,
		precondition func(existingObject Object) error,
		updateFunc func(existingObject Object) (Object, error)) error
	// Delete deletes the object from the storage layer for the given key
	// For Future use, when etcd storage is implemented
	Delete(ctx context.Context, key string) error
}
