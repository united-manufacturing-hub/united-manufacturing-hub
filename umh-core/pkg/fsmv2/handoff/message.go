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

package handoff

// Message carries a received message for the router in pkg/communicator/router
// to consume.
type Message struct {
	// Content holds the message bytes as received. The caller must not modify
	// Content after handing the Message over.
	Content []byte
	// SenderEmail is the sender the message names. SenderEmail is not proof
	// of who sent the message.
	SenderEmail string
	// TraceID is the trace ID as a UUID in string form, where an empty or
	// unparseable value means no trace.
	TraceID string
}
