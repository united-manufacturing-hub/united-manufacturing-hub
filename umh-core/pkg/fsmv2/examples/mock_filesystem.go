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
	"context"
	"io/fs"
	"os"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// mockFilesystem is an in-memory filesystem.Service. Only ReadFile is
// implemented; any other method panics on the nil embedded Service.
type mockFilesystem struct {
	filesystem.Service

	mu    sync.Mutex
	files map[string][]byte
}

func (m *mockFilesystem) SetFile(path, contents string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.files[path] = []byte(contents)
}

func (m *mockFilesystem) RemoveFile(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.files, path)
}

func (m *mockFilesystem) ReadFile(_ context.Context, path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	contents, ok := m.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: path, Err: os.ErrNotExist}
	}

	out := make([]byte, len(contents))
	copy(out, contents)

	return out, nil
}
