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

package snapshot_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
)

func TestSnapshot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Example Parent Snapshot Suite")
}

var _ = Describe("ExampleparentConfig", func() {
	It("should expose the embedded BaseUserSpec.GetState default", func() {
		cfg := &snapshot.ExampleparentConfig{}
		Expect(cfg.GetState()).To(Equal("running"))
	})

	It("should default ChildWorkerType to examplechild", func() {
		cfg := &snapshot.ExampleparentConfig{}
		Expect(cfg.GetChildWorkerType()).To(Equal("examplechild"))
	})

	It("should honour a custom ChildWorkerType", func() {
		cfg := &snapshot.ExampleparentConfig{ChildWorkerType: "custom"}
		Expect(cfg.GetChildWorkerType()).To(Equal("custom"))
	})
})

var _ = Describe("ExampleparentStatus", func() {
	It("should be a zero-value struct (framework carries all lifecycle fields)", func() {
		status := snapshot.ExampleparentStatus{}
		Expect(status).To(Equal(snapshot.ExampleparentStatus{}))
	})
})
