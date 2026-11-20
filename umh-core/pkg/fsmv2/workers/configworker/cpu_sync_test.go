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

package configworker_test

import (
	"context"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"
)

// withClient returns the registry behind a freshly published client, which is
// deliberately NOT the one newConstructedWorker hands the worker. So the
// child appears there only if the worker went through the client; a reconcile
// writing its own registry directly would fail the upsert test.
func withClient(t *testing.T) *dynamicchildren.Registry {
	t.Helper()

	dynWriter := dynamicchildren.NewWriter()
	fsmv2client.SetClient(fsmv2client.NewFSMv2Client(dynWriter, nil))
	t.Cleanup(func() { fsmv2client.SetClient(nil) })

	return dynWriter.Registry()
}

func TestCollectObservedStateUpsertsCPU(t *testing.T) {
	w := newConstructedWorker(t)
	clientShared := withClient(t)

	desired := &fsmv2.WrappedDesiredState[snapshot.ConfigworkerConfig]{}
	if _, err := w.CollectObservedState(context.Background(), desired); err != nil {
		t.Fatalf("CollectObservedState: %v", err)
	}

	if !clientShared.Contains(fsmv2cpu.Ref) {
		t.Fatalf("registry does not contain %v after reconcile", fsmv2cpu.Ref)
	}
}
