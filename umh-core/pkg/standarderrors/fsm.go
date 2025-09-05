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

package standarderrors

import "errors"

var (
	// ErrInstanceRemoved is returned when an instance has been successfully removed.
	ErrInstanceRemoved = errors.New("instance removed")

	// ErrRemovalPending is returned by RemoveBenthosFromS6Manager while the
	// S6 manager is still busy shutting the service down.  Callers should
	// treat it as a *retryable* error.
	ErrRemovalPending = errors.New("service removal still in progress")
)
