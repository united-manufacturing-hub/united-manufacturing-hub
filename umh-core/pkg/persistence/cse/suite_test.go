package cse_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCSE(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CSE Suite")
}
