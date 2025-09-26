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

package http

// Endpoint represents the HTTP path type used for API endpoints.
type Endpoint string

var (
	// LoginEndpoint is the login endpoint for instance authentication.
	LoginEndpoint Endpoint = "/v2/instance/login"
	// PushEndpoint is the push endpoint for sending instance data.
	PushEndpoint Endpoint = "/v2/instance/push"
	// PullEndpoint is the pull endpoint for retrieving instance data.
	PullEndpoint Endpoint = "/v2/instance/pull"
)
