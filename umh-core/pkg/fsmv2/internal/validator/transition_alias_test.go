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

package validator_test

import (
	"go/parser"
	"go/token"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/validator"
)

var _ = Describe("FindLegacyResultCalls (TransitionAlias matcher)", func() {

	It("flags fsmv2.Result[any, any](...) as a legacy call", func() {
		src := `package sample

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

func Next() any {
	return fsmv2.Result[any, any](nil, fsmv2.SignalNone, nil, "reason")
}
`

		fset := token.NewFileSet()

		file, err := parser.ParseFile(fset, "test.go", src, 0)
		Expect(err).NotTo(HaveOccurred())

		hits := validator.FindLegacyResultCalls(file, fset)
		Expect(hits).To(HaveLen(1))
		Expect(hits[0].Symbol).To(Equal("fsmv2.Result"))
		Expect(hits[0].Pos.Line).To(Equal(6))
	})

	It("flags fsmv2.WrapAction[MyDeps](...) as a legacy call", func() {
		src := `package sample

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

type MyDeps struct{}

func Wrap() any {
	return fsmv2.WrapAction[MyDeps](nil)
}
`

		fset := token.NewFileSet()

		file, err := parser.ParseFile(fset, "test.go", src, 0)
		Expect(err).NotTo(HaveOccurred())

		hits := validator.FindLegacyResultCalls(file, fset)
		Expect(hits).To(HaveLen(1))
		Expect(hits[0].Symbol).To(Equal("fsmv2.WrapAction"))
	})

	It("respects custom import aliases and renders the alias in Symbol", func() {
		src := `package sample

import v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

func Next() any {
	return v2.Result[any, any](nil, v2.SignalNone, nil, "reason")
}
`

		fset := token.NewFileSet()

		file, err := parser.ParseFile(fset, "test.go", src, 0)
		Expect(err).NotTo(HaveOccurred())

		hits := validator.FindLegacyResultCalls(file, fset)
		Expect(hits).To(HaveLen(1))
		Expect(hits[0].Symbol).To(Equal("v2.Result"))
	})

	It("does not flag fsmv2.Transition(...) calls", func() {
		src := `package sample

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

func Next() any {
	return fsmv2.Transition(nil, fsmv2.SignalNone, nil, "reason")
}
`

		fset := token.NewFileSet()

		file, err := parser.ParseFile(fset, "test.go", src, 0)
		Expect(err).NotTo(HaveOccurred())

		hits := validator.FindLegacyResultCalls(file, fset)
		Expect(hits).To(BeEmpty())
	})

	It("does not flag fsmv2.Result(...) without explicit type parameters", func() {
		src := `package sample

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

func Next() any {
	return fsmv2.Result(nil, fsmv2.SignalNone, nil, "reason")
}
`

		fset := token.NewFileSet()

		file, err := parser.ParseFile(fset, "test.go", src, 0)
		Expect(err).NotTo(HaveOccurred())

		hits := validator.FindLegacyResultCalls(file, fset)
		Expect(hits).To(BeEmpty())
	})

	It("returns nil for files that do not import fsmv2", func() {
		src := `package sample

import "fmt"

func Print() {
	fmt.Println("hello")
}
`

		fset := token.NewFileSet()

		file, err := parser.ParseFile(fset, "test.go", src, 0)
		Expect(err).NotTo(HaveOccurred())

		hits := validator.FindLegacyResultCalls(file, fset)
		Expect(hits).To(BeNil())
	})

	It("returns multiple hits with distinct positions", func() {
		src := `package sample

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

func Next() any {
	_ = fsmv2.Result[any, any](nil, fsmv2.SignalNone, nil, "first")
	_ = fsmv2.WrapAction[struct{}](nil)
	return nil
}
`

		fset := token.NewFileSet()

		file, err := parser.ParseFile(fset, "test.go", src, 0)
		Expect(err).NotTo(HaveOccurred())

		hits := validator.FindLegacyResultCalls(file, fset)
		Expect(hits).To(HaveLen(2))
		Expect(hits[0].Symbol).To(Equal("fsmv2.Result"))
		Expect(hits[1].Symbol).To(Equal("fsmv2.WrapAction"))
		Expect(hits[0].Pos.Line).NotTo(Equal(hits[1].Pos.Line))
	})
})
