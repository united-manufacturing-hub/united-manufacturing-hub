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
)

// SimpleAction creates an Action[any] from a typed function.
// Handles ctx cancellation automatically before invoking fn.
// Eliminates the need for action structs in simple cases.
//
// The returned action type-asserts depsAny to TDeps at call time.
// A wrong type returns a descriptive error.
func SimpleAction[TDeps any](name string, fn func(ctx context.Context, deps TDeps) error) Action[any] {
	return &simpleAction[TDeps]{name: name, fn: fn}
}

type simpleAction[TDeps any] struct {
	fn   func(ctx context.Context, deps TDeps) error
	name string
}

func (a *simpleAction[TDeps]) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	typedDeps, ok := depsAny.(TDeps)
	if !ok {
		return fmt.Errorf("SimpleAction %q: expected deps type %T, got %T", a.name, *new(TDeps), depsAny)
	}

	return a.fn(ctx, typedDeps)
}

func (a *simpleAction[TDeps]) Name() string   { return a.name }
func (a *simpleAction[TDeps]) String() string { return a.name }
