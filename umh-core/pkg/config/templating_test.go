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

package config

import (
	"context"
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"

	dataflowcomponentserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
)

var _ = Describe("Data-flow component anchor protection", func() {
	// ── YAML fixture: one anchored, one plain ──────────────────────────
	const yamlWithAnchors = `
templates:
  - &tpl
    benthos:
      input:
        generate:
          mapping: root = "hello"
          interval: 1s
          count: 0
      output:
        stdout: {}

dataFlow:
  - name: dfc-anchored
    desiredState: active
    dataFlowComponentConfig: *tpl

  - name: dfc-plain
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        input:
          generate:
            mapping: root = "world"
            interval: 1s
            count: 0
        output:
          stdout: {}
`
	var (
		ctx          context.Context
		mockFS       *filesystem.MockFileSystem
		cfgMgr       *FileConfigManager
		plainUUID    uuid.UUID
		anchoredUUID uuid.UUID
		plainDFC     DataFlowComponentConfig
		anchoredDFC  DataFlowComponentConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockFS = filesystem.NewMockFileSystem()

		// default behaviours
		mockFS.WithEnsureDirectoryFunc(func(_ context.Context, _ string) error { return nil })
		mockFS.WithFileExistsFunc(func(_ context.Context, _ string) (bool, error) { return true, nil })
		mockFS.WithReadFileFunc(func(_ context.Context, _ string) ([]byte, error) {
			return []byte(yamlWithAnchors), nil
		})
		mockFS.WithWriteFileFunc(func(_ context.Context, _ string, _ []byte, _ os.FileMode) error {
			// succeed silently
			return nil
		})

		cfgMgr = NewFileConfigManager().WithFileSystemService(mockFS)

		// preload config once to grab DFC structs and UUIDs
		cfg, err := cfgMgr.GetConfig(ctx, 0)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.DataFlow).To(HaveLen(2))
		if cfg.DataFlow[0].Name == "dfc-anchored" {
			anchoredDFC = cfg.DataFlow[0]
			plainDFC = cfg.DataFlow[1]
		} else {
			plainDFC = cfg.DataFlow[0]
			anchoredDFC = cfg.DataFlow[1]
		}
		anchoredUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(anchoredDFC.Name)
		plainUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(plainDFC.Name)
	})

	Describe("flagging anchors on unmarshal", func() {
		It("sets hasAnchors only on the anchored DFC", func() {
			Expect(anchoredDFC.HasAnchors()).To(BeTrue())
			Expect(plainDFC.HasAnchors()).To(BeFalse())
		})
	})

	Describe("AtomicEditDataflowcomponent", func() {
		It("rejects edit when the target DFC contains anchors", func() {
			_, err := cfgMgr.AtomicEditDataflowcomponent(ctx, anchoredUUID, anchoredDFC)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("anchors/aliases"))
		})

		It("allows edit when the DFC is plain", func() {
			_, err := cfgMgr.AtomicEditDataflowcomponent(ctx, plainUUID, plainDFC)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
