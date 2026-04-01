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

package protocol_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/protocol"
)

var _ = Describe("Detector", func() {
	var det protocol.Detector

	BeforeEach(func() {
		det = protocol.NewDetector(zap.NewNop().Sugar())
	})

	Describe("Detect", func() {
		It("returns the v0 handler for messages without a protocol version", func() {
			msg := &transport.UMHMessage{
				Content: `eyJNZXNzYWdlVHlwZSI6InN1YnNjcmliZSJ9`,
				Email:   "test@example.com",
			}
			handler := det.Detect(msg)
			out, err := handler.Decrypt([]byte("hello"), "test@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(string(out)).To(Equal("hello"))
		})

		It("returns the cse_v1 handler for messages with CseV1 protocol version", func() {
			msg := &transport.UMHMessage{
				Content:         `eyJNZXNzYWdlVHlwZSI6InN1YnNjcmliZSJ9`,
				Email:           "test@example.com",
				ProtocolVersion: transport.CseV1,
			}
			handler := det.Detect(msg)
			_, err := handler.Decrypt([]byte("hello"), "test@example.com")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not yet implemented"))
		})
	})
})
