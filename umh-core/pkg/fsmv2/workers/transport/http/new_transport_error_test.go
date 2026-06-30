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

package transport

import (
	"strings"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("newTransportError body capture", func() {
	It("includes the first 256 bytes of a large body in the message", func() {
		fLarge := "<html><body>" + strings.Repeat("A", 4000) + "</body></html>"
		te := newTransportError(502, []byte(fLarge), nil, nil)
		Expect(te.Message).To(ContainSubstring(fLarge[:100]))
		Expect(te.Message).NotTo(Equal("HTTP 502: server_error"))
		Expect(te.Message).NotTo(ContainSubstring(strings.Repeat("A", 257)))
	})

	It("keeps the short message when the body is empty", func() {
		te := newTransportError(502, nil, nil, nil)
		Expect(te.Message).To(Equal("HTTP 502: server_error"))
	})

	It("includes a 220-byte body that the old <200 gate would have dropped", func() {
		body := strings.Repeat("a", 220)
		te := newTransportError(502, []byte(body), nil, nil)
		Expect(te.Message).To(ContainSubstring(body))
	})

	It("caps a multibyte body on a rune boundary so the message stays valid UTF-8", func() {
		// 100 Euro signs = 300 bytes; the 256-byte cap lands mid-rune.
		te := newTransportError(502, []byte(strings.Repeat("€", 100)), nil, nil)
		Expect(utf8.ValidString(te.Message)).To(BeTrue())
	})
})
