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

// GetAreas returns all areas of a site
func GetAreas(enterpriseName string, siteName string) (areas []string, err error) {
	zap.S().Infof("[GetAreas] Getting areas for enterprise %s and site %s", enterpriseName, siteName)

	areas = []string{models.MockDefaultArea}

	return
}
