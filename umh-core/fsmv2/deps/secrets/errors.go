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

// ErrZeroKey is returned by Get, Put and Delete for a zero Key, one that NewKey
// did not build.
var ErrZeroKey = errors.New("secrets: zero Key; declare keys with NewKey")

// ForWorker returns ErrInvalidPath when the hierarchy path is empty.
var ErrInvalidPath = errors.New("secrets: invalid hierarchy path")

// ErrNotFound is returned by Get when nothing is stored under the key in the
// scope's path.
var ErrNotFound = errors.New("secrets: entry not found")

// ErrNoStore is returned by ForWorker when the handle has no store behind it,
// and by Get, Put and Delete on a zero Scope, such as the one ForWorker returns
// with an error.
var ErrNoStore = errors.New("secrets: no store")
