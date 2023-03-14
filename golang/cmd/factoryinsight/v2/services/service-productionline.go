// Copyright 2023 UMH Systems GmbH
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

package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"go.uber.org/zap"
)

// GetProductionLines returns all production lines for a given area
func GetProductionLines(
	enterpriseName string,
	siteName string,
	areaName string,
) (productionLines []string, err error) {

	zap.S().Infof(
		"[GetProductionLines] Getting production lines for enterprise %s, site %s and area %s",
		enterpriseName,
		siteName,
		areaName,
	)

	productionLines = []string{models.MockDefaultProductionLine}

	return
}
