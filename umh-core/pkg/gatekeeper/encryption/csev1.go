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

import (
	"errors"

	"go.uber.org/zap"
)

// CseV1Handler handles client-side encryption v1.
// TODO(ENG-4627): Add cryptolib build tag split when MC cryptolib is integrated.
type CseV1Handler struct {
	log *zap.SugaredLogger
}

// NewCseV1Handler creates a CSE v1 handler.
func NewCseV1Handler(log *zap.SugaredLogger) Handler {
	return &CseV1Handler{log: log}
}

func (h *CseV1Handler) Decrypt(_ []byte, _ string) ([]byte, error) {
	return nil, errors.New("cseV1 not yet implemented")
}

func (h *CseV1Handler) Encrypt(_ []byte, _ string) ([]byte, error) {
	return nil, errors.New("cseV1 not yet implemented")
}
