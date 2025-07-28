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

package actions

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

func filterLogsByTimestamp(logs []s6.LogEntry, configChangeAt time.Time) []s6.LogEntry {
	result := make([]s6.LogEntry, 0)
	for _, log := range logs {
		if log.Timestamp.After(configChangeAt) {
			result = append(result, log)
		}
	}
	return result
}
