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

package generator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

var _ = Describe("DataModelsFromConfig latest version", func() {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	buildConfigManager := func(keys []string) config.ConfigManager {
		versions := map[string]config.DataModelVersion{}
		for _, k := range keys {
			versions[k] = config.DataModelVersion{}
		}

		return config.NewMockConfigManager().WithConfig(config.FullConfig{
			DataModels: []config.DataModelsConfig{
				{Name: "pump", Versions: versions},
			},
		})
	}

	DescribeTable("picks the highest version",
		func(keys []string, want string) {
			manager := buildConfigManager(keys)

			out, err := DataModelsFromConfig(ctx, manager, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(HaveLen(1))
			Expect(out[0].LatestVersion).To(Equal(want))
		},
		Entry("legacy only", []string{"v1"}, "v1"),
		Entry("legacy plus minor", []string{"v1", "v1_1"}, "v1_1"),
		Entry("all two-part", []string{"v1_0", "v1_1"}, "v1_1"),
		Entry("two majors", []string{"v1", "v2"}, "v2"),
		Entry("numeric major order", []string{"v9", "v10"}, "v10"),
		Entry("minor beyond nine", []string{"v1_9", "v1_10"}, "v1_10"),
	)

	It("skips an unparseable key and reports the best good one", func() {
		manager := buildConfigManager([]string{"not-a-version", "v1", "v1_1"})

		out, err := DataModelsFromConfig(ctx, manager, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(HaveLen(1))
		Expect(out[0].LatestVersion).To(Equal("v1_1"))
	})

	It("reports an empty LatestVersion when no key parses", func() {
		manager := buildConfigManager([]string{"not-a-version", "also-bad"})

		out, err := DataModelsFromConfig(ctx, manager, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(HaveLen(1))
		Expect(out[0].LatestVersion).To(BeEmpty())
	})

	It("is stable across repeated calls", func() {
		manager := buildConfigManager([]string{"v1_0", "v1_1", "v1_2", "v1_3", "v1_4", "v1_5", "v1_6", "v1_7"})

		for range 200 {
			out, err := DataModelsFromConfig(ctx, manager, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(out[0].LatestVersion).To(Equal("v1_7"))
		}
	})
})
