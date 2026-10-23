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

package secrets_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/secrets"
)

// fakeDeps stands in for a worker's dependencies in these rows.
type fakeDeps struct {
	path string
}

func (f fakeDeps) GetHierarchyPath() string {
	return f.path
}

// Test paths.
const (
	pathC = "app(application)/communicator(communicator)"
	pathT = pathC + "/transport(transport)"
	pathF = pathC + "/certfetcher(certfetcher)"
)

// The real path form: a root application worker, a numbered communicator
// child, and a numbered transport child under it.
const (
	realC = "application-fsmv2(application)/communicator-001(communicator)"
	realT = realC + "/transport-001(transport)"
)

// open returns a scope from ScopeOf, failing the row if it errors.
func open(h secrets.Handle, path string) secrets.Scope {
	scope, err := secrets.ScopeOf(h, fakeDeps{path})
	Expect(err).NotTo(HaveOccurred())
	return scope
}

var _ = Describe("Scope", func() {
	It("S1 checks that a scope from ScopeOf stores a value with Put and Get returns the same value", func() {
		store := secrets.NewMemoryStore()
		scope := open(store.Handle(), pathT)

		Expect(scope.Put("k", secrets.NewValue("v1"))).To(Succeed())
		val, err := scope.Get("k")
		Expect(err).NotTo(HaveOccurred())
		Expect(val.Reveal()).To(Equal("v1"))
	})

	It("S1b checks that a scope at the real path form stores a value with Put and Get returns the same value", func() {
		store := secrets.NewMemoryStore()
		scope := open(store.Handle(), realT)

		Expect(scope.Put("k", secrets.NewValue("v1"))).To(Succeed())
		val, err := scope.Get("k")
		Expect(err).NotTo(HaveOccurred())
		Expect(val.Reveal()).To(Equal("v1"))
	})

	It("S2 checks that ScopeOf refuses an empty or malformed path and the store stays empty", func() {
		store := secrets.NewMemoryStore()
		h := store.Handle()

		for _, malformed := range []string{"", "/a(x)", "a(x)/", "a(x)//b(y)", "a(x)/../b(y)", "."} {
			scope, err := secrets.ScopeOf(h, fakeDeps{malformed})
			Expect(err).NotTo(BeNil(), "ScopeOf(%q)", malformed)
			Expect(errors.Is(err, secrets.ErrInvalidOwner)).To(BeTrue(), "ScopeOf(%q)", malformed)

			err = scope.Put("k", secrets.NewValue("v"))
			Expect(err).NotTo(BeNil(), "Put on the scope from ScopeOf(%q)", malformed)
		}

		// The store holds no entry after the refused puts.
		Expect(secrets.EntryCount(store)).To(Equal(0))
	})

	It("S3 checks that ScopeOf returns ErrNoStore for a handle with no store behind it", func() {
		scope, err := secrets.ScopeOf(secrets.Handle{}, fakeDeps{pathT})
		Expect(err).NotTo(BeNil())
		Expect(errors.Is(err, secrets.ErrNoStore)).To(BeTrue())
		Expect(scope.Put("k", secrets.NewValue("v"))).NotTo(Succeed())
	})

	It("S4 checks that a scope opened from a worker's own dependencies does not read a sibling's entries", func() {
		store := secrets.NewMemoryStore()
		h := store.Handle()
		transport := open(h, pathT)
		fetcher := open(h, pathF)

		Expect(transport.Put("jwt", secrets.NewValue("v1"))).To(Succeed())

		_, err := fetcher.Get("jwt")
		Expect(errors.Is(err, secrets.ErrNotFound)).To(BeTrue(), "fetcher Get")

		comm, ok := fetcher.Ancestor("communicator")
		Expect(ok).To(BeTrue())
		_, err = comm.Get("jwt")
		Expect(errors.Is(err, secrets.ErrNotFound)).To(BeTrue(), "fetcher's communicator Get")
	})

	It("S5 checks that Ancestor finds the scope of an ancestor of a given worker type", func() {
		store := secrets.NewMemoryStore()
		h := store.Handle()
		commScope := open(h, pathC)
		transport := open(h, pathT)

		Expect(commScope.Put("auth_token", secrets.NewValue("v1"))).To(Succeed())

		comm, ok := transport.Ancestor("communicator")
		Expect(ok).To(BeTrue())
		val, err := comm.Get("auth_token")
		Expect(err).NotTo(HaveOccurred())
		Expect(val.Reveal()).To(Equal("v1"))
	})

	It("S5b checks that Ancestor finds an ancestor's entry at the real path form", func() {
		store := secrets.NewMemoryStore()
		h := store.Handle()
		commScope := open(h, realC)
		transport := open(h, realT)

		Expect(commScope.Put("auth_token", secrets.NewValue("v1"))).To(Succeed())

		comm, ok := transport.Ancestor("communicator")
		Expect(ok).To(BeTrue())
		val, err := comm.Get("auth_token")
		Expect(err).NotTo(HaveOccurred())
		Expect(val.Reveal()).To(Equal("v1"))
	})

	It("S6 checks that an ancestor scope can write and delete that ancestor's entries", func() {
		store := secrets.NewMemoryStore()
		h := store.Handle()
		transport := open(h, pathT)
		fetcher := open(h, pathF)

		commT, ok := transport.Ancestor("communicator")
		Expect(ok).To(BeTrue())
		Expect(commT.Put("jwt", secrets.NewValue("v1"))).To(Succeed())

		commF, ok := fetcher.Ancestor("communicator")
		Expect(ok).To(BeTrue())
		val, err := commF.Get("jwt")
		Expect(err).NotTo(HaveOccurred())
		Expect(val.Reveal()).To(Equal("v1"))

		Expect(commT.Delete("jwt")).To(Succeed())

		_, err = commF.Get("jwt")
		Expect(errors.Is(err, secrets.ErrNotFound)).To(BeTrue())
	})

	It("S7 checks that Ancestor resolves to the nearest ancestor of the given worker type", func() {
		store := secrets.NewMemoryStore()
		h := store.Handle()
		scope := open(h, "a(x)/b(x)/c(y)")

		anc, ok := scope.Ancestor("x")
		Expect(ok).To(BeTrue())

		// b's write is visible through the ancestor scope, a's is not.
		a := open(h, "a(x)")
		b := open(h, "a(x)/b(x)")
		Expect(a.Put("k", secrets.NewValue("from-a"))).To(Succeed())
		Expect(b.Put("k", secrets.NewValue("from-b"))).To(Succeed())

		val, err := anc.Get("k")
		Expect(err).NotTo(HaveOccurred())
		Expect(val.Reveal()).To(Equal("from-b"))
	})

	It("S8 checks that Ancestor never falls back to the worker's own path, the root or a prefix match", func() {
		store := secrets.NewMemoryStore()
		h := store.Handle()

		transport := open(h, "app(application)/transport(transport)")
		_, ok := transport.Ancestor("transport")
		Expect(ok).To(BeFalse(), "the own segment never matches")
		_, ok = transport.Ancestor("communicator")
		Expect(ok).To(BeFalse())
		_, ok = transport.Ancestor("")
		Expect(ok).To(BeFalse())

		suffixed := open(h, "app(application)/c(communicatorx)/t(transport)")
		_, ok = suffixed.Ancestor("communicator")
		Expect(ok).To(BeFalse(), "the type part must match exactly")

		emptyType := open(h, "app(application)/x()/t(transport)")
		_, ok = emptyType.Ancestor("")
		Expect(ok).To(BeFalse(), "an empty type is refused before the loop")
	})

	It("S9 checks that an invalid entry name is refused by Get, Put and Delete and nothing is written", func() {
		store := secrets.NewMemoryStore()
		scope := open(store.Handle(), pathT)

		for _, name := range []string{"", "a/b", ".", ".."} {
			_, err := scope.Get(name)
			Expect(errors.Is(err, secrets.ErrInvalidName)).To(BeTrue(), "Get with name "+name)

			err = scope.Put(name, secrets.NewValue("v"))
			Expect(errors.Is(err, secrets.ErrInvalidName)).To(BeTrue(), "Put with name "+name)

			err = scope.Delete(name)
			Expect(errors.Is(err, secrets.ErrInvalidName)).To(BeTrue(), "Delete with name "+name)
		}

		_, err := scope.Get("k")
		Expect(errors.Is(err, secrets.ErrNotFound)).To(BeTrue(), "nothing was written")
	})

	It("S10 checks that two scopes can Put and Get concurrently", func() {
		store := secrets.NewMemoryStore()
		h := store.Handle()
		putter := open(h, pathT)
		getter := open(h, pathT)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				_ = putter.Put("k", secrets.NewValue("v"))
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				_, _ = getter.Get("k")
			}
		}()
		wg.Wait()
	})

	It("S11 checks that the handle travels through the dependency map and still opens the same store", func() {
		store := secrets.NewMemoryStore()
		commScope := open(store.Handle(), pathC)
		Expect(commScope.Put("auth_token", secrets.NewValue("v1"))).To(Succeed())

		root := map[string]any{}
		config.SetDependency(root, secrets.StoreKey, store.Handle())
		child := map[string]any{"other": 1}
		merged := config.MergeDependencies(root, child)

		h, ok := config.LookupDependency(merged, secrets.StoreKey)
		Expect(ok).To(BeTrue())

		scope := open(h, pathC)
		val, err := scope.Get("auth_token")
		Expect(err).NotTo(HaveOccurred())
		Expect(val.Reveal()).To(Equal("v1"))
	})

	It("S12 checks that the held string reaches no saved state through any rendering", func() {
		store := secrets.NewMemoryStore()
		scope := open(store.Handle(), pathC)
		Expect(scope.Put("k", secrets.NewValue("PLANTED-S12"))).To(Succeed())

		handle := store.Handle()
		handlePtr := &handle
		depMap := map[string]any{}
		config.SetDependency(depMap, secrets.StoreKey, handle)
		spec := config.ChildSpec{Dependencies: depMap}

		objects := []struct {
			name    string
			value   any
			marshal func(any) ([]byte, error)
		}{
			{"handle", handle, json.Marshal},
			{"&handle", handlePtr, json.Marshal},
			{"dependency map", depMap, json.Marshal},
			{"ChildSpec", spec, json.Marshal},
		}
		for _, obj := range objects {
			b, err := obj.marshal(obj.value)
			Expect(err).NotTo(HaveOccurred(), "json.Marshal of "+obj.name)
			Expect(string(b)).NotTo(ContainSubstring("PLANTED-S12"), "json.Marshal of "+obj.name)

			for verb, out := range map[string]string{
				"%v":  fmt.Sprintf("%v", obj.value),
				"%+v": fmt.Sprintf("%+v", obj.value),
				"%#v": fmt.Sprintf("%#v", obj.value),
				"%s":  fmt.Sprintf("%s", obj.value),
			} {
				Expect(out).NotTo(ContainSubstring("PLANTED-S12"), verb+" of "+obj.name)
			}
		}

		// Positive control: the same renderings of a plain string field do
		// contain the value.
		probe := struct{ S string }{"PLANTED-S12"}
		b, err := json.Marshal(probe)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(b)).To(ContainSubstring("PLANTED-S12"))
		Expect(fmt.Sprintf("%v", probe)).To(ContainSubstring("PLANTED-S12"))
		Expect(fmt.Sprintf("%+v", probe)).To(ContainSubstring("PLANTED-S12"))
		Expect(fmt.Sprintf("%#v", probe)).To(ContainSubstring("PLANTED-S12"))
		Expect(fmt.Sprintf("%s", probe)).To(ContainSubstring("PLANTED-S12"))
	})
})
