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

// Package encryption provides protocol-specific encryption and decryption handlers.
package encryption

// Handler handles encryption/decryption for a specific protocol version.
type Handler interface {
	Decrypt(data []byte) ([]byte, error)
	Encrypt(data []byte) ([]byte, error)
}
