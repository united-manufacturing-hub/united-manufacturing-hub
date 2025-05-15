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

package testutil

import (
	"os"
	"time"
)

// MemFileInfo is a simple implementation of os.FileInfo for testing
type MemFileInfo struct {
	FileName  string
	FileSize  int64
	FileMode  os.FileMode
	FileMtime time.Time
	FileDir   bool
}

func (m *MemFileInfo) Name() string       { return m.FileName }
func (m *MemFileInfo) Size() int64        { return m.FileSize }
func (m *MemFileInfo) Mode() os.FileMode  { return m.FileMode }
func (m *MemFileInfo) ModTime() time.Time { return m.FileMtime }
func (m *MemFileInfo) IsDir() bool        { return m.FileDir }
func (m *MemFileInfo) Sys() interface{}   { return nil }
