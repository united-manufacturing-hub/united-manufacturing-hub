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

package types_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// fNginx is a real nginx 502 HTML page with CRLF between tags; the double-quoted
// literal makes \r\n real control bytes, exercising sanitizeErrorDetail's
// control-strip path.
const fNginx = "<html>\r\n<head><title>502 Bad Gateway</title></head>\r\n<body>\r\n<center><h1>502 Bad Gateway</h1></center>\r\n<hr><center>nginx</center>\r\n</body>\r\n</html>\r\n"

var _ = Describe("ExtractErrorDetails", func() {
	It("returns the status code and sanitized body of a TransportError", func() {
		te := &types.TransportError{
			Err:        errors.New("upstream 502"),
			Message:    "HTTP 502 (server_error): " + fNginx,
			Type:       types.ErrorTypeServerError,
			StatusCode: 502,
			RetryAfter: 0,
		}

		et, ra, sc, detail := types.ExtractErrorDetails(te)
		Expect(et).To(Equal(types.ErrorTypeServerError))
		Expect(ra).To(BeZero())
		Expect(sc).To(Equal(502))
		Expect(detail).To(ContainSubstring("502 Bad Gateway"))
		Expect(detail).To(ContainSubstring("nginx"))
		Expect(detail).ToNot(ContainSubstring("\r"))
		Expect(detail).ToNot(ContainSubstring("\n"))
	})

	It("recovers a TransportError wrapped in an error chain (caller contract: error_helpers_test unwraps fmt.Errorf %w chains)", func() {
		te := &types.TransportError{
			Err:        errors.New("upstream 502"),
			Message:    "HTTP 502 (server_error): " + fNginx,
			Type:       types.ErrorTypeServerError,
			StatusCode: 502,
			RetryAfter: 0,
		}
		wrapped := fmt.Errorf("push failed: %w", te)

		et, ra, sc, detail := types.ExtractErrorDetails(wrapped)
		Expect(et).To(Equal(types.ErrorTypeServerError))
		Expect(ra).To(BeZero())
		Expect(sc).To(Equal(502))
		Expect(detail).To(ContainSubstring("502 Bad Gateway"))
	})

	It("returns zero status code and a sanitized message for a plain error", func() {
		et, ra, sc, detail := types.ExtractErrorDetails(errors.New("dial tcp: connection\r\nrefused"))
		Expect(et).To(Equal(types.ErrorTypeUnknown))
		Expect(ra).To(BeZero())
		Expect(sc).To(Equal(0))
		Expect(detail).To(ContainSubstring("connection"))
		Expect(detail).To(ContainSubstring("refused"))
		Expect(detail).ToNot(ContainSubstring("\r"))
		Expect(detail).ToNot(ContainSubstring("\n"))
	})

	It("returns zero values for nil without panicking", func() {
		et, ra, sc, detail := types.ExtractErrorDetails(nil)
		Expect(et).To(Equal(types.ErrorTypeUnknown))
		Expect(ra).To(BeZero())
		Expect(sc).To(Equal(0))
		Expect(detail).To(BeEmpty())
	})

	It("returns zero values for a typed-nil wrapped TransportError without panicking", func() {
		var te *types.TransportError
		wrapped := fmt.Errorf("wrap: %w", te)

		et, ra, sc, detail := types.ExtractErrorDetails(wrapped)
		Expect(et).To(Equal(types.ErrorTypeUnknown))
		Expect(ra).To(BeZero())
		Expect(sc).To(Equal(0))
		Expect(detail).To(BeEmpty())

		// ExtractErrorType delegates to ExtractErrorDetails, so it must not
		// panic on a typed-nil-wrapped value either.
		et2, ra2 := types.ExtractErrorType(wrapped)
		Expect(et2).To(Equal(types.ErrorTypeUnknown))
		Expect(ra2).To(BeZero())
	})
})
