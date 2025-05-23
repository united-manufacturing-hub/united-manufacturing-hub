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

package dataflowcomponent

import "errors"

// Custom errors for DataFlowComponent service
var (
	// ErrServiceNotExists is returned when a dataflow component does not exist
	ErrServiceNotExists = errors.New("dataflow component does not exist")
	// ErrServiceAlreadyExists is returned when a dataflow component already exists
	ErrServiceAlreadyExists = errors.New("dataflow component already exists")
)
