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

package monitor

import (
	"fmt"
	"strings"
)

func ParseCurlError(errorString string) error {
	if !strings.Contains(errorString, "curl") {
		return nil
	}

	knownErrors := map[string]error{
		"curl: (7)":  ErrServiceConnectionRefused,  // Failed to connect to host (often “Connection refused” or “No route to host”)	TCP handshake never completed
		"curl: (28)": ErrServiceConnectionTimedOut, // Operation timed out	Connect or first-byte timeout (server never answered)
		"curl: (52)": ErrServiceConnectionRefused,  // Empty reply from server	TCP established but peer closed without sending any HTTP bytes
		"curl: (55)": ErrServiceConnectionRefused,  // Send failure	Kernel reported ECONNRESET while writing request bytes
		"curl: (56)": ErrServiceConnectionRefused,  // 	Recv failure: connection reset by peer	TCP reset while reading the response
	}

	for knownError, err := range knownErrors {
		if strings.Contains(errorString, knownError) {
			return err
		}
	}

	return fmt.Errorf("unknown curl error: %s", errorString)
}
