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

package cpuhealth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("a cancelled tick", func() {
	It("fails the read rather than reporting a box with no cgroup", func() {
		// filesystem.DefaultService checks the context, so a cancelled tick
		// fails every read. That is the same shape as a host with none of these
		// files, and without this the sampler reports the second when it saw
		// the first: an instance shutting down would look unmeasurable.
		const base = "/sys/fs/cgroup"

		fs := filesystem.NewMockFileSystem()
		fs.ReadFileFunc = func(ctx context.Context, _ string) ([]byte, error) {
			return nil, ctx.Err()
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := cpuhealth.NewLinuxSampler(fs, base).Read(ctx)
		Expect(err).To(MatchError(context.Canceled))
	})
})
