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

package timescalemetrics

import "strings"

// historianTablePrefixes and historianTableNames name what benthos-umh's
// historian output creates: two hypertables per data contract, plus the shared
// lookup tables.
var historianTablePrefixes = []string{"value_", "attribute_"}

var historianTableNames = map[string]bool{"tag": true, "topic": true, "location": true}

func historianCreated(name string) bool {
	if historianTableNames[name] {
		return true
	}

	for _, prefix := range historianTablePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// historianTables drops every table in the schema this product did not create: a
// customer's own table alongside ours says nothing about how the historian is
// doing, and its columns would all read as absent.
func historianTables(tables []Table) []Table {
	kept := make([]Table, 0, len(tables))

	for _, table := range tables {
		if historianCreated(table.Name) {
			kept = append(kept, table)
		}
	}

	return kept
}
