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

package s6

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// healthyFS returns a MockFileSystem where every file/path probe reports the
// resource as present. Used as the baseline for "all tracked files exist".
func healthyFS() *filesystem.MockFileSystem {
	fs := filesystem.NewMockFileSystem()
	fs.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
		return true, nil
	})
	fs.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
		return true, nil
	})

	return fs
}

var _ = Describe("Health() recovery directive contract", func() {
	var (
		ctx         context.Context
		service     *DefaultService
		servicePath string
		repoPath    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		service = &DefaultService{logger: logger.For("test")}
		servicePath = filepath.Join(constants.S6BaseDir, "test-service")
		repoPath = filepath.Join(constants.GetS6RepositoryBaseDir(), "test-service")
	})

	It("returns ActionOK when no artifacts are tracked (post-cleanup or pre-Create)", func() {
		service.artifacts.Store(nil)

		Expect(service.Health(ctx, healthyFS())).To(Equal(ActionOK))
	})

	It("returns ActionOK when all tracked files exist on disk", func() {
		service.artifacts.Store(&ServiceArtifacts{
			ServiceDir:    servicePath,
			RepositoryDir: repoPath,
			CreatedFiles: []string{
				filepath.Join(repoPath, "run"),
				filepath.Join(repoPath, "type"),
			},
		})

		Expect(service.Health(ctx, healthyFS())).To(Equal(ActionOK))
	})

	It("returns ActionRecreate when tracked files are missing (corruption observed)", func() {
		service.artifacts.Store(&ServiceArtifacts{
			ServiceDir:    servicePath,
			RepositoryDir: repoPath,
			CreatedFiles: []string{
				filepath.Join(repoPath, "run"),
				filepath.Join(repoPath, "type"),
			},
		})

		fs := filesystem.NewMockFileSystem()
		fs.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
			if strings.HasSuffix(path, "/run") {
				return false, nil
			}

			return true, nil
		})
		fs.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
			return true, nil
		})

		Expect(service.Health(ctx, fs)).To(Equal(ActionRecreate))
	})

	It("returns ActionWait on transient I/O error from filesystem", func() {
		service.artifacts.Store(&ServiceArtifacts{
			ServiceDir:    servicePath,
			RepositoryDir: repoPath,
			CreatedFiles:  []string{filepath.Join(repoPath, "run")},
		})

		fs := filesystem.NewMockFileSystem()
		fs.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
			return false, errors.New("I/O timeout")
		})

		Expect(service.Health(ctx, fs)).To(Equal(ActionWait))
	})

	It("returns ActionWait while RemoveArtifacts is in flight (RemovalProgress != nil)", func() {
		artifacts := &ServiceArtifacts{
			ServiceDir:    servicePath,
			RepositoryDir: repoPath,
			CreatedFiles:  []string{filepath.Join(repoPath, "run")},
		}
		artifacts.InitRemovalProgress()
		service.artifacts.Store(artifacts)

		fs := filesystem.NewMockFileSystem()
		fs.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
			return false, nil
		})

		Expect(service.Health(ctx, fs)).To(Equal(ActionWait))
	})

	It("flips ActionRecreate → ActionOK after cleanup clears tracking", func() {
		artifacts := &ServiceArtifacts{
			ServiceDir:    servicePath,
			RepositoryDir: repoPath,
			CreatedFiles:  []string{filepath.Join(repoPath, "run")},
		}
		service.artifacts.Store(artifacts)

		brokenFS := filesystem.NewMockFileSystem()
		brokenFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
			return false, nil
		})
		brokenFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
			return true, nil
		})
		Expect(service.Health(ctx, brokenFS)).To(Equal(ActionRecreate))

		service.artifacts.Store(nil)
		Expect(service.Health(ctx, brokenFS)).To(Equal(ActionOK))
	})

	It("flips ActionOK → ActionRecreate when files are externally deleted", func() {
		artifacts := &ServiceArtifacts{
			ServiceDir:    servicePath,
			RepositoryDir: repoPath,
			CreatedFiles:  []string{filepath.Join(repoPath, "run")},
		}
		service.artifacts.Store(artifacts)

		Expect(service.Health(ctx, healthyFS())).To(Equal(ActionOK))

		brokenFS := filesystem.NewMockFileSystem()
		brokenFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
			return false, nil
		})
		brokenFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
			return true, nil
		})
		Expect(service.Health(ctx, brokenFS)).To(Equal(ActionRecreate))
	})
})
