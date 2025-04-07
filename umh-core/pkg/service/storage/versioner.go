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

package storage

import (
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
)

var _ Versioner = &SimpleVersioner{}

// SimpleVersioner is a versioner that implements a numeric versioning scheme
type SimpleVersioner struct {
	counter uint64
}

// NewSimpleVersioner creates a new SimpleVersioner
func NewSimpleVersioner() *SimpleVersioner {
	return &SimpleVersioner{
		counter: 0,
	}
}

// CompareResourceVersions compares two resource versions
// Returns -1 if lhs is less than rhs, 0 if they are equal, and 1 if lhs is greater than rhs
func (s SimpleVersioner) CompareResourceVersions(lhs string, rhs string) int {

	lhsVersion, lErr := s.ParseResourceVersion(lhs)
	rhsVersion, rErr := s.ParseResourceVersion(rhs)

	if lErr != nil && rErr != nil {
		return 0
	}

	if lErr != nil {
		return -1
	}

	if rErr != nil {
		return 1
	}

	if lhsVersion < rhsVersion {
		return -1
	}

	if lhsVersion > rhsVersion {
		return 1
	}

	return 0
}

// UpdateVersion updates the version of the object
func (s SimpleVersioner) UpdateVersion(object Object) error {
	if object == nil {
		return errors.New("object cannot be nil while updating version")
	}

	newVersion := atomic.AddUint64(&s.counter, 1)
	object.SetResourceVersion(fmt.Sprintf("%d", newVersion))
	return nil
}

// ParseResourceVersion parses a string resource version to uint64
func (v *SimpleVersioner) ParseResourceVersion(resourceVersion string) (uint64, error) {
	if resourceVersion == "" {
		return 0, nil
	}
	return strconv.ParseUint(resourceVersion, 10, 64)
}
