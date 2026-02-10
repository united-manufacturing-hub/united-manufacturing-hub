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

package sentry

import "strings"

// HierarchyInfo contains parsed information from a hierarchy path.
type HierarchyInfo struct {
	FSMVersion  string
	WorkerType  string
	WorkerChain string
}

// ParseHierarchyPath parses a hierarchy path string and extracts FSM version, worker type,
// and worker chain.
//
// FSMv2 format: "app(application)/comm-001(communicator)" - uses parentheses.
// FSMv1 format: "Enterprise.Site.Area.Line.WorkCell" - uses dots.
//
// WorkerChain extracts only the type from each segment, omitting instance IDs:
//
//	"app(application)/comm-001(communicator)/pull-001(pull)" -> "application/communicator/pull"
//
// This provides the full hierarchy context without customer-specific data (instance names,
// bridge names) that would cause tag cardinality issues in Sentry.
func ParseHierarchyPath(path string) HierarchyInfo {
	info := HierarchyInfo{FSMVersion: "unknown", WorkerType: "unknown"}

	if path == "" {
		return info
	}

	if strings.Contains(path, "(") {
		info.FSMVersion = "v2"
		segments := strings.Split(path, "/")
		var types []string
		for _, seg := range segments {
			if start := strings.Index(seg, "("); start != -1 {
				if end := strings.Index(seg, ")"); end > start {
					if t := seg[start+1 : end]; t != "" {
						types = append(types, t)
					}
				}
			}
		}
		if len(types) > 0 {
			info.WorkerType = types[len(types)-1]
			info.WorkerChain = strings.Join(types, "/")
		}
	} else {
		info.FSMVersion = "v1"

		segments := strings.Split(path, ".")
		if len(segments) > 0 {
			info.WorkerType = segments[len(segments)-1]
			info.WorkerChain = strings.Join(segments, "/")
		}
	}

	return info
}
