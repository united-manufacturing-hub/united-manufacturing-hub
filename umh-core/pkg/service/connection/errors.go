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

package connection

import "errors"

var (
	// ErrServiceNotExist indicates the requested service does not exist
	ErrServiceNotExist = errors.New("connection service does not exist")
	// ErrServiceAlreadyExists indicates the service already exists
	ErrServiceAlreadyExists = errors.New("connection service already exists")

	// ErrRemovalPending is **not** a failure.
	// It means the desired config has been deleted but the child FSM still
	// exists.  Callers should keep reconciling and retry the removal in the
	// next cycle.
	ErrRemovalPending = errors.New("service removal still in progress")
)
