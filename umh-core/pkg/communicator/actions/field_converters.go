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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// modelsRelationalToConfig converts models.RelationalShape to config.PayloadShape.
func modelsRelationalToConfig(rel *models.RelationalShape) *config.PayloadShape {
	if rel == nil {
		return nil
	}

	configFields := make(map[string]config.PayloadField, len(rel.Fields))
	for name, field := range rel.Fields {
		configFields[name] = config.PayloadField{Type: field.Type}
	}

	return &config.PayloadShape{Fields: configFields}
}

// configRelationalToModels converts config.PayloadShape to models.RelationalShape.
func configRelationalToModels(ps *config.PayloadShape) *models.RelationalShape {
	if ps == nil {
		return nil
	}

	relFields := make(map[string]models.RelationalField, len(ps.Fields))
	for name, field := range ps.Fields {
		relFields[name] = models.RelationalField{Type: field.Type}
	}

	return &models.RelationalShape{Fields: relFields}
}
