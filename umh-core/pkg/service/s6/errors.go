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

package s6

import "errors"

var (
	// ErrServiceNotExist indicates the requested service does not exist
	ErrServiceNotExist = errors.New("service does not exist")

	// ErrServiceConfigMapNotFound indicates the service config map was not found
	ErrServiceConfigMapNotFound = errors.New("service config map not found")

	// ErrInvalidStatus is returned when the status file has an invalid format
	ErrInvalidStatus = errors.New("invalid status file format")
)
