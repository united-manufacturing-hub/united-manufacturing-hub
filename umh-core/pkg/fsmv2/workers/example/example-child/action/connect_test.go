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
	action := NewConnectAction()
	var depsAny any = newMockDeps()

	err := action.Execute(context.Background(), depsAny)
	if err != nil {
		t.Errorf("Execute() error = %v, want nil", err)
	}
}

func TestConnectAction_Execute_WithFailures(t *testing.T) {
	// Note: Retry logic will be handled by ActionExecutor in Phase 2C
	// For now, this test just verifies the action can be created
	action := NewConnectActionWithFailures(2)
	var depsAny any = newMockDeps()

	// All executions succeed (skeleton implementation)
	err1 := action.Execute(context.Background(), depsAny)
	if err1 != nil {
		t.Errorf("Execute() error = %v, want nil (skeleton implementation)", err1)
	}

	err2 := action.Execute(context.Background(), depsAny)
	if err2 != nil {
		t.Errorf("Execute() error = %v, want nil (skeleton implementation)", err2)
	}

	err3 := action.Execute(context.Background(), depsAny)
	if err3 != nil {
		t.Errorf("Execute() error = %v, want nil (skeleton implementation)", err3)
	}
}

func TestConnectAction_Name(t *testing.T) {
	action := NewConnectAction()

	if action.Name() != ConnectActionName {
		t.Errorf("Name() = %v, want %v", action.Name(), ConnectActionName)
	}
}
