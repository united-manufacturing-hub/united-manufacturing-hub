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

package router

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("resubscribedFromPayload", func() {
	It("reads resubscribed from a decoded JSON payload", func() {
		resubscribed, err := resubscribedFromPayload(map[string]interface{}{"resubscribed": true})

		Expect(err).NotTo(HaveOccurred())
		Expect(resubscribed).To(BeTrue())
	})

	It("reports a new subscriber when the payload says so", func() {
		resubscribed, err := resubscribedFromPayload(map[string]interface{}{"resubscribed": false})

		Expect(err).NotTo(HaveOccurred())
		Expect(resubscribed).To(BeFalse())
	})

	It("reports a new subscriber for a payload without the field", func() {
		resubscribed, err := resubscribedFromPayload(map[string]interface{}{})

		Expect(err).NotTo(HaveOccurred())
		Expect(resubscribed).To(BeFalse())
	})

	It("reports a new subscriber for an absent payload", func() {
		resubscribed, err := resubscribedFromPayload(nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(resubscribed).To(BeFalse())
	})

	It("errors on a payload that is not a decoded object", func() {
		_, err := resubscribedFromPayload("resubscribed")

		Expect(err).To(HaveOccurred())
	})

	It("still reads an already-typed payload", func() {
		resubscribed, err := resubscribedFromPayload(models.SubscribeMessagePayload{Resubscribed: true})

		Expect(err).NotTo(HaveOccurred())
		Expect(resubscribed).To(BeTrue())
	})
})
