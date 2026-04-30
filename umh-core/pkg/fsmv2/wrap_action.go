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

package fsmv2

import (
	"context"
	"fmt"
	"reflect"
)

// WrapAction adapts a typed action (with concrete TDeps) into Action[any]
// for use in the ActionProvider interface. The type assertion from any to
// TDeps happens at execution time; a mismatch panics with a descriptive message.
func WrapAction[TDeps any](typed interface {
	Execute(context.Context, TDeps) error
	Name() string
},
) Action[any] {
	return &wrappedAction[TDeps]{typed: typed}
}

// wrappedAction bridges a typed action to the Action[any] interface.
type wrappedAction[TDeps any] struct {
	typed interface {
		Execute(context.Context, TDeps) error
		Name() string
	}
}

// Execute type-asserts deps from any to TDeps and delegates to the typed action.
func (w *wrappedAction[TDeps]) Execute(ctx context.Context, deps any) error {
	typedDeps, ok := deps.(TDeps)
	if !ok {
		panic(fmt.Sprintf(
			"WrapAction[%s].Execute: deps type assertion failed: expected %s, got %T",
			w.typed.Name(), reflect.TypeFor[TDeps](), deps,
		))
	}

	return w.typed.Execute(ctx, typedDeps)
}

// Name delegates to the typed action.
func (w *wrappedAction[TDeps]) Name() string {
	return w.typed.Name()
}
