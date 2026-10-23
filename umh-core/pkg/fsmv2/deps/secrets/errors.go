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

package secrets

import "errors"

// ErrInvalidName is returned when an entry name is empty or contains a
// separator or path element.
var ErrInvalidName = errors.New("secrets: invalid entry name")

// ErrInvalidOwner is returned when a hierarchy path is empty or malformed.
var ErrInvalidOwner = errors.New("secrets: invalid owner path")

// ErrNotFound is returned when no entry of that name is stored under the
// scope's path.
var ErrNotFound = errors.New("secrets: entry not found")

// ErrNoStore is returned by ScopeOf when the handle has no store behind it,
// because nothing ever put one in the dependency map.
var ErrNoStore = errors.New("secrets: no store")
