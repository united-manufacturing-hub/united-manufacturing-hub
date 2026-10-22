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

package examples

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// newMockFilesystem returns a filesystem mock whose ReadFile, WriteFile and
// Remove work on an in-memory map of files. Reading a missing file returns
// the error os.ReadFile returns. Every other method is the mock's default.
func newMockFilesystem() *filesystem.MockFileSystem {
	var (
		mu    sync.Mutex
		files = map[string][]byte{}
	)

	return filesystem.NewMockFileSystem().
		WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			mu.Lock()
			defer mu.Unlock()

			contents, ok := files[path]
			if !ok {
				return nil, &fs.PathError{Op: "read", Path: path, Err: os.ErrNotExist}
			}

			return bytes.Clone(contents), nil
		}).
		WithWriteFileFunc(func(_ context.Context, path string, data []byte, _ os.FileMode) error {
			mu.Lock()
			defer mu.Unlock()

			files[path] = bytes.Clone(data)

			return nil
		}).
		WithRemoveFunc(func(_ context.Context, path string) error {
			mu.Lock()
			defer mu.Unlock()

			delete(files, path)

			return nil
		})
}
