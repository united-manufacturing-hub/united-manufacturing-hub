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

// Package certificatehandler manages user certificate caching, retrieval, and
// fetching from the Management Console API.
package certificatehandler

import (
	"context"
	"crypto/x509"
)

// Handler manages certificate caching, retrieval, and fetching.
type Handler interface {
	Certificate(email string) *x509.Certificate
	IntermediateCerts(email string) []*x509.Certificate
	RootCA() *x509.Certificate
	FetchAndStore(ctx context.Context, email string) error
	SetJWT(jwt string)
	SetInstanceUUID(uuid string)
}
