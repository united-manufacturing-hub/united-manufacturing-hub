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
	FSMVersion string
	WorkerType string
}

// ParseHierarchyPath parses a hierarchy path string and extracts FSM version and worker type.
// FSMv2 format: "app(application)/worker(communicator)" - uses parentheses.
// FSMv1 format: "Enterprise.Site.Area.Line.WorkCell" - uses dots.
func ParseHierarchyPath(path string) HierarchyInfo {
	info := HierarchyInfo{FSMVersion: "unknown", WorkerType: "unknown"}

	if path == "" {
		return info
	}

	if strings.Contains(path, "(") {
		info.FSMVersion = "v2"
		// Extract last segment's type: "app(application)/worker(communicator)" -> "communicator"
		segments := strings.Split(path, "/")
		if len(segments) > 0 {
			lastSegment := segments[len(segments)-1]
			if start := strings.Index(lastSegment, "("); start != -1 {
				if end := strings.Index(lastSegment, ")"); end > start {
					info.WorkerType = lastSegment[start+1 : end]
				}
			}
		}
	} else {
		info.FSMVersion = "v1"

		segments := strings.Split(path, ".")
		if len(segments) > 0 {
			info.WorkerType = segments[len(segments)-1]
		}
	}

	return info
}
