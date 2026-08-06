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

// This file exposes unexported seams of the datamodel package to the external
// datamodel_test package. It is compiled into test binaries only.

package datamodel

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// FlattenResolvedForTest exposes flattenResolved to the external test
// package. The returned map's value type (resolvedTag) is itself unexported,
// so callers can hold it via type inference and read its ShapeName and Shape
// fields, but cannot name the type.
func FlattenResolvedForTest(
	ctx context.Context,
	version config.DataModelVersion,
	allModels map[string]config.DataModelsConfig,
	payloadShapes map[string]config.PayloadShape,
) (map[string]resolvedTag, error) {
	return flattenResolved(ctx, version, allModels, payloadShapes)
}

// ShapesEqualForTest exposes shapesEqual to the external test package.
func ShapesEqualForTest(a, b config.PayloadShape) bool {
	return shapesEqual(a, b)
}
