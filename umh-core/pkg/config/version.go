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

package config

import (
	"fmt"
	"regexp"
	"strconv"
)

// VersionKeyPattern is the grammar for a data model version key: "v1" or "v1_2".
// A bare "vN" means major N, minor 0. Neither part may carry a leading zero,
// except that the minor part alone may be the literal "0".
var VersionKeyPattern = regexp.MustCompile(`^v([1-9]\d*)(?:_(0|[1-9]\d*))?$`)

// Version is a data model version key decomposed into its parts.
type Version struct {
	Major int
	Minor int
}

// ParseVersion reads a version key. A bare "vN" is major N, minor 0.
func ParseVersion(key string) (Version, error) {
	m := VersionKeyPattern.FindStringSubmatch(key)
	if m == nil {
		return Version{}, fmt.Errorf("version key %q must match ^v[1-9]\\d*(_(0|[1-9]\\d*))?$", key)
	}

	major, err := strconv.Atoi(m[1])
	if err != nil {
		return Version{}, fmt.Errorf("version key %q has an unreadable major: %w", key, err)
	}

	minor := 0
	if m[2] != "" {
		minor, err = strconv.Atoi(m[2])
		if err != nil {
			return Version{}, fmt.Errorf("version key %q has an unreadable minor: %w", key, err)
		}
	}

	return Version{Major: major, Minor: minor}, nil
}

// String renders the canonical two-part form, so a Version always round-trips.
func (v Version) String() string {
	return fmt.Sprintf("v%d_%d", v.Major, v.Minor)
}

// Compare orders by major, then minor. It returns a negative number if v is
// lower than o, zero if they are equal, and a positive number if v is higher.
func (v Version) Compare(o Version) int {
	if v.Major != o.Major {
		return v.Major - o.Major
	}

	return v.Minor - o.Minor
}

// NextMinor returns the key to write for a new version: the highest major's
// highest minor, plus one. An empty set of keys yields v1_0. An unreadable key
// is an error rather than something to skip, so a version can never be
// renumbered around.
func NextMinor(keys []string) (Version, error) {
	if len(keys) == 0 {
		return Version{Major: 1, Minor: 0}, nil
	}

	var highest Version

	for _, key := range keys {
		v, err := ParseVersion(key)
		if err != nil {
			return Version{}, fmt.Errorf("cannot compute the next version: %w", err)
		}

		if v.Compare(highest) > 0 {
			highest = v
		}
	}

	return Version{Major: highest.Major, Minor: highest.Minor + 1}, nil
}
