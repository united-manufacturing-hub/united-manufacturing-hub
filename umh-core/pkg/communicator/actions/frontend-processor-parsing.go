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
	"strconv"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// this function returns weather the string indexes of the given map are all numeric
// this func is only called when len(pipeline) > 0 so we dont need to check
func CheckIfOrderedNumericKeys(pipeline map[string]models.DfcDataConfig) bool {
	for processorName := range pipeline {
		index, err := strconv.Atoi(processorName)
		if err != nil || index < 0 {
			return false
		}
	}
	return true
}
