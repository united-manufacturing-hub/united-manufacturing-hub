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

package action

import (
	"context"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"go.uber.org/zap"
)

type mockDeps struct {
	*fsmv2.BaseDependencies
}

func newMockDeps() *mockDeps {
	return &mockDeps{
		BaseDependencies: fsmv2.NewBaseDependencies(zap.NewNop().Sugar()),
	}
}

func TestConnectAction_Execute_Success(t *testing.T) {
	deps := newMockDeps()
	action := NewConnectAction(deps)

	err := action.Execute(context.Background())
	if err != nil {
		t.Errorf("Execute() error = %v, want nil", err)
	}
}

func TestConnectAction_Execute_WithFailures(t *testing.T) {
	deps := newMockDeps()
	action := NewConnectActionWithFailures(deps, 2)

	err1 := action.Execute(context.Background())
	if err1 == nil {
		t.Error("First Execute() should fail")
	}

	err2 := action.Execute(context.Background())
	if err2 == nil {
		t.Error("Second Execute() should fail")
	}

	err3 := action.Execute(context.Background())
	if err3 != nil {
		t.Errorf("Third Execute() error = %v, want nil", err3)
	}
}

func TestConnectAction_Name(t *testing.T) {
	deps := newMockDeps()
	action := NewConnectAction(deps)

	if action.Name() != ConnectActionName {
		t.Errorf("Name() = %v, want %v", action.Name(), ConnectActionName)
	}
}
