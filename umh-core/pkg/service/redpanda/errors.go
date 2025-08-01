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

package redpanda

import (
	"errors"
	"fmt"
)

var (
	// ErrServiceNotExist indicates the requested service does not exist
	ErrServiceNotExist = errors.New("service does not exist")

	// ErrServiceAlreadyExists indicates the requested service already exists
	ErrServiceAlreadyExists = errors.New("service already exists")

	// ErrServiceNoLogFile indicates the health check had no logs to process
	ErrServiceNoLogFile = errors.New("log file not found")

	// ErrRedpandaMonitorNotRunning indicates the redpanda monitor service is not running
	ErrRedpandaMonitorNotRunning = errors.New("redpanda monitor service is not running")

	// ErrRedpandaMonitorInstanceNotFound indicates the redpanda monitor instance was not found
	ErrRedpandaMonitorInstanceNotFound = errors.New("instance redpanda-monitor not found")

	// ErrLastObservedStateNil indicates the last observed state is nil
	ErrLastObservedStateNil = errors.New("last observed state is nil")
)

// SchemaRegistryError wraps errors that originate from schema registry operations
type SchemaRegistryError struct {
	Err error
}

func (e *SchemaRegistryError) Error() string {
	return fmt.Sprintf("schema registry error: %v", e.Err)
}

func (e *SchemaRegistryError) Unwrap() error {
	return e.Err
}

// WrapSchemaRegistryError wraps an error as a schema registry error
func WrapSchemaRegistryError(err error) error {
	if err == nil {
		return nil
	}
	return &SchemaRegistryError{Err: err}
}

// IsSchemaRegistryError checks if an error is a schema registry error
func IsSchemaRegistryError(err error) bool {
	var schemaRegistryErr *SchemaRegistryError
	return errors.As(err, &schemaRegistryErr)
}
