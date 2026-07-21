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

package actions_test

import (
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// historianReplyStates decodes the action-reply state of every message captured on
// the outbound channel, in order. The historian action tests use it to assert on the
// reply protocol Execute emits — not just the return value — so a regression that
// self-sends a terminal reply (letting the dispatcher send a second one) is caught.
func historianReplyStates(messages *[]*models.UMHMessage, mu *sync.Mutex) []models.ActionReplyState {
	mu.Lock()
	defer mu.Unlock()

	states := make([]models.ActionReplyState, 0, len(*messages))

	for _, msg := range *messages {
		decoded, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
		if err != nil {
			continue
		}

		payload, ok := decoded.Payload.(map[string]interface{})
		if !ok {
			continue
		}

		if state, ok := payload["actionReplyState"].(string); ok {
			states = append(states, models.ActionReplyState(state))
		}
	}

	return states
}
