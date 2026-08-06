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

package datamodel

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// resolvedTag is one leaf tag path with the payload shape it resolves to.
// Shape carries the definition rather than only the name, because a relational
// field's synthetic shape name is derived from its path and so stays identical
// even when the fields underneath it change.
type resolvedTag struct {
	ShapeName string
	Shape     config.PayloadShape
}

// flattenResolved reduces a data model version to its leaf tag paths with each
// path's resolved payload shape.
//
// payloadShapes is enriched with the three built-in default shapes before
// resolution, because a real config.yaml never carries a payloadShapes
// section: without the defaults, "timeseries-number" and "timeseries-string"
// would both miss the map and resolve to the zero config.PayloadShape, which
// compare equal to each other. A shape name that still fails to resolve after
// enrichment is an error rather than the zero shape, for the same reason: two
// unresolvable names must never compare equal.
//
// allModels is always resolved through, even when nil: a nil map lookup
// returns the zero value and false, so a version with a _refModel field
// errors instead of silently vanishing from the output when no model set is
// supplied. That vanishing is otherwise indistinguishable from a genuine tag
// removal to a caller comparing two flattened versions.
//
// Each call enriches its own copy of payloadShapes, so two versions flattened
// from the same caller-supplied map cannot overwrite each other's synthetic
// relational shape entries.
func flattenResolved(
	ctx context.Context,
	version config.DataModelVersion,
	allModels map[string]config.DataModelsConfig,
	payloadShapes map[string]config.PayloadShape,
) (map[string]resolvedTag, error) {
	shapes := ensureDefaultPayloadShapes(payloadShapes)

	translator := NewTranslator()

	pathsByShape, err := translator.extractVirtualPathsWithReferences(
		ctx, version.Structure, "", allModels, shapes, make(map[string]bool), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to flatten data model version: %w", err)
	}

	out := make(map[string]resolvedTag, len(pathsByShape))

	for shapeName, paths := range pathsByShape {
		shape, exists := shapes[shapeName]
		if !exists {
			return nil, fmt.Errorf("payload shape %q is not defined", shapeName)
		}

		for _, path := range paths {
			out[path] = resolvedTag{ShapeName: shapeName, Shape: shape}
		}
	}

	return out, nil
}

// shapesEqual reports whether two payload shape definitions are structurally
// identical. It compares fields, not names, because a relational field's
// synthetic shape name is derived from its path and stays identical even when
// the underlying definition changes.
func shapesEqual(a, b config.PayloadShape) bool {
	return payloadFieldsEqual(a.Fields, b.Fields)
}

func payloadFieldsEqual(a, b map[string]config.PayloadField) bool {
	if len(a) != len(b) {
		return false
	}

	for name, fieldA := range a {
		fieldB, exists := b[name]
		if !exists {
			return false
		}

		if fieldA.Type != fieldB.Type {
			return false
		}

		if !payloadFieldsEqual(fieldA.Subfields, fieldB.Subfields) {
			return false
		}
	}

	return true
}

// BreakingKind is why a change is not additive.
type BreakingKind int

const (
	// Removed means a tag path present in the previous version is gone.
	Removed BreakingKind = iota
	// Retyped means a tag path survived but its payload shape changed.
	Retyped
)

// BreakingChange is one reason a candidate version is not additive.
type BreakingChange struct {
	Path     string
	OldShape string
	NewShape string
	Kind     BreakingKind
}

// CheckAdditive reports every way in which next fails to be a purely additive
// successor of prev. An empty result means every tag in prev survives in next
// with the same resolved payload shape, which is what the Historian's fixed
// column types depend on.
func CheckAdditive(
	ctx context.Context,
	prev, next config.DataModelVersion,
	allModels map[string]config.DataModelsConfig,
	payloadShapes map[string]config.PayloadShape,
) ([]BreakingChange, error) {
	prevTags, err := flattenResolved(ctx, prev, allModels, payloadShapes)
	if err != nil {
		return nil, fmt.Errorf("cannot read the previous version: %w", err)
	}

	nextTags, err := flattenResolved(ctx, next, allModels, payloadShapes)
	if err != nil {
		return nil, fmt.Errorf("cannot read the candidate version: %w", err)
	}

	paths := make([]string, 0, len(prevTags))
	for path := range prevTags {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	changes := make([]BreakingChange, 0)

	for _, path := range paths {
		before := prevTags[path]

		after, survives := nextTags[path]
		if !survives {
			changes = append(changes, BreakingChange{
				Path:     path,
				Kind:     Removed,
				OldShape: before.ShapeName,
			})

			continue
		}

		if !shapesEqual(before.Shape, after.Shape) {
			changes = append(changes, BreakingChange{
				Path:     path,
				Kind:     Retyped,
				OldShape: before.ShapeName,
				NewShape: after.ShapeName,
			})
		}
	}

	return changes, nil
}

// FormatBreakingChanges renders a refusal that names the rule and says the
// escape hatch does not exist, so the reader does not go looking for one.
func FormatBreakingChanges(modelName, versionKey string, changes []BreakingChange) string {
	var b strings.Builder

	fmt.Fprintf(&b, "cannot add version %s to data model %q: %d breaking change", versionKey, modelName, len(changes))

	if len(changes) != 1 {
		b.WriteString("s")
	}

	b.WriteString("\n\n")

	for _, change := range changes {
		switch change.Kind {
		case Removed:
			fmt.Fprintf(&b, "  %s  removed (was %s)\n", change.Path, change.OldShape)
		case Retyped:
			fmt.Fprintf(&b, "  %s  payload shape changed: %s -> %s\n", change.Path, change.OldShape, change.NewShape)
		}
	}

	b.WriteString("\nA new minor version may only add tags. Changing or removing an existing tag\n")
	b.WriteString("requires a new major version, which is not supported yet.")

	return b.String()
}
