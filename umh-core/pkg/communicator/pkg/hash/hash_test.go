package hash_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/hash"
)

var _ = Describe("Go Hash", func() {
	It("validates go hasher returns the same as JS sha3_256 hasher", func() {
		testInput := "ABC"
		expectedOutput := "7fb50120d9d1bc7504b4b7f1888d42ed98c0b47ab60a20bd4a2da7b2c1360efa"

		actualOutput := hash.Sha3Hash(testInput)

		Expect(actualOutput).To(Equal(expectedOutput))
	})
})
