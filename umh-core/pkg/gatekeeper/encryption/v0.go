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

package encryption

import "go.uber.org/zap"

// V0Handler is a no-op encryption handler for the legacy v0 protocol.
type V0Handler struct {
	log *zap.SugaredLogger
}

// NewV0Handler creates a no-op encryption handler for the legacy v0 protocol.
func NewV0Handler(log *zap.SugaredLogger) Handler {
	return &V0Handler{log: log}
}

func (h *V0Handler) Decrypt(data []byte) ([]byte, error) {
	return data, nil
}

func (h *V0Handler) Encrypt(data []byte) ([]byte, error) {
	return data, nil
}
