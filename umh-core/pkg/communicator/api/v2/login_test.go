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

package v2

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("Login", func() {
	// This token belongs to a community user, that is created exactly for this test and has no other machines or permissions
	token := "ff2dbf482c7e54dbaecc9367d56b93cf492653da075345155991d5fad6fa5e4e"
	It("should hash the auth token correctly", func() {

		result := LoginHash(token)

		Expect(result).NotTo(BeEmpty())
		Expect(result).To(HaveLen(64)) // SHA3-256 produces 64 character hex string
	})

	It("should login successfully against management.umh.app", func() {
		logger := zap.NewNop().Sugar()

		response, err := login(token, false, "https://management.umh.app/api", logger)

		Expect(err).NotTo(HaveOccurred())
		Expect(response).NotTo(BeNil())
		Expect(response.JWT).NotTo(BeEmpty())
		Expect(response.UUID).NotTo(BeNil())
		Expect(response.Name).NotTo(BeEmpty())
	})

})
