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

// fakeDeps stands in for a worker's dependencies in these tests.
type fakeDeps struct {
	path string
}

func (f fakeDeps) GetHierarchyPath() string {
	return f.path
}

const (
	communicatorPath = "app(application)/communicator(communicator)"
	transportPath    = communicatorPath + "/transport(transport)"
	certFetcherPath  = communicatorPath + "/certfetcher(certfetcher)"

	// The form the supervisor builds: a root application worker, a numbered
	// communicator child, and a numbered transport child under it.
	numberedTransportPath = "application-fsmv2(application)/communicator-001(communicator)/transport-001(transport)"
)

var (
	tokenKey = secrets.NewKey("test.token")
	otherKey = secrets.NewKey("test.other")
)

// open returns the scope ForWorker gives for path, failing the test if it errors.
func open(handle secrets.Handle, path string) secrets.Scope {
	scope, err := handle.ForWorker(fakeDeps{path})
	Expect(err).NotTo(HaveOccurred())

	return scope
}

var _ = Describe("Scope", func() {
	DescribeTable("stores a value with Put and returns it with Get",
		func(path string) {
			store := secrets.NewStore()
			scope := open(store.Handle(), path)

			Expect(scope.Put(tokenKey, secrets.NewValue("v1"))).To(Succeed())
			value, err := scope.Get(tokenKey)
			Expect(err).NotTo(HaveOccurred())
			Expect(value.Reveal()).To(Equal("v1"))
		},
		Entry("at a short path", transportPath),
		Entry("at the numbered path form", numberedTransportPath),
	)

	It("returns ErrNotFound from Get for a key that nothing was stored under", func() {
		store := secrets.NewStore()
		scope := open(store.Handle(), transportPath)
		Expect(scope.Put(tokenKey, secrets.NewValue("v1"))).To(Succeed())

		_, err := scope.Get(otherKey)
		Expect(errors.Is(err, secrets.ErrNotFound)).To(BeTrue())
	})

	It("removes an entry with Delete", func() {
		store := secrets.NewStore()
		scope := open(store.Handle(), transportPath)
		Expect(scope.Put(tokenKey, secrets.NewValue("v1"))).To(Succeed())

		Expect(scope.Delete(tokenKey)).To(Succeed())

		_, err := scope.Get(tokenKey)
		Expect(errors.Is(err, secrets.ErrNotFound)).To(BeTrue())
		Expect(secrets.EntryCount(store)).To(Equal(0))

		Expect(scope.Delete(otherKey)).To(Succeed(), "Delete of a key with no entry")
	})

	It("gives each worker its own entries: a sibling's scope does not see them", func() {
		store := secrets.NewStore()
		handle := store.Handle()
		transport := open(handle, transportPath)
		certFetcher := open(handle, certFetcherPath)
		communicator := open(handle, communicatorPath)

		Expect(transport.Put(tokenKey, secrets.NewValue("v1"))).To(Succeed())

		_, err := certFetcher.Get(tokenKey)
		Expect(errors.Is(err, secrets.ErrNotFound)).To(BeTrue(), "sibling Get")
		_, err = communicator.Get(tokenKey)
		Expect(errors.Is(err, secrets.ErrNotFound)).To(BeTrue(), "parent Get")

		again := open(handle, transportPath)
		value, err := again.Get(tokenKey)
		Expect(err).NotTo(HaveOccurred(), "a second scope for the same path")
		Expect(value.Reveal()).To(Equal("v1"))
	})

	It("refuses an empty or malformed path in ForWorker with ErrInvalidPath, and the store stays empty", func() {
		store := secrets.NewStore()
		handle := store.Handle()

		for _, malformed := range []string{"", "/a(x)", "a(x)/", "a(x)//b(y)", "a(x)/../b(y)", "a(x)/./b(y)", "."} {
			scope, err := handle.ForWorker(fakeDeps{malformed})
			Expect(errors.Is(err, secrets.ErrInvalidPath)).To(BeTrue(), "ForWorker(%q)", malformed)

			err = scope.Put(tokenKey, secrets.NewValue("v"))
			Expect(errors.Is(err, secrets.ErrNoStore)).To(BeTrue(), "Put on the scope from ForWorker(%q)", malformed)
		}

		Expect(secrets.EntryCount(store)).To(Equal(0))
	})

	It("returns ErrNoStore from ForWorker for a handle with no store behind it", func() {
		_, err := secrets.Handle{}.ForWorker(fakeDeps{transportPath})
		Expect(errors.Is(err, secrets.ErrNoStore)).To(BeTrue())
	})

	It("returns ErrNoStore from Get, Put and Delete on a zero Scope", func() {
		var scope secrets.Scope

		_, err := scope.Get(tokenKey)
		Expect(errors.Is(err, secrets.ErrNoStore)).To(BeTrue(), "Get")
		err = scope.Put(tokenKey, secrets.NewValue("v"))
		Expect(errors.Is(err, secrets.ErrNoStore)).To(BeTrue(), "Put")
		err = scope.Delete(tokenKey)
		Expect(errors.Is(err, secrets.ErrNoStore)).To(BeTrue(), "Delete")
	})

	It("returns ErrInvalidName from Get, Put and Delete for a Key that NewKey did not build, writes nothing, and accepts a key from NewKey", func() {
		store := secrets.NewStore()
		scope := open(store.Handle(), transportPath)

		_, err := scope.Get(secrets.Key{})
		Expect(errors.Is(err, secrets.ErrInvalidName)).To(BeTrue(), "Get")
		err = scope.Put(secrets.Key{}, secrets.NewValue("v"))
		Expect(errors.Is(err, secrets.ErrInvalidName)).To(BeTrue(), "Put")
		err = scope.Delete(secrets.Key{})
		Expect(errors.Is(err, secrets.ErrInvalidName)).To(BeTrue(), "Delete")

		Expect(secrets.EntryCount(store)).To(Equal(0))

		// The same calls with a key from NewKey succeed.
		Expect(scope.Put(tokenKey, secrets.NewValue("v"))).To(Succeed(), "Put with a valid key")
		_, err = scope.Get(tokenKey)
		Expect(err).NotTo(HaveOccurred(), "Get with a valid key")
		Expect(scope.Delete(tokenKey)).To(Succeed(), "Delete with a valid key")
	})

	It("lets two scopes Put and Get concurrently without a data race (run with -race)", func() {
		store := secrets.NewStore()
		handle := store.Handle()
		putter := open(handle, transportPath)
		getter := open(handle, transportPath)

		var wg sync.WaitGroup

		wg.Add(2)
		go func() {
			defer wg.Done()

			for range 1000 {
				_ = putter.Put(tokenKey, secrets.NewValue("v"))
			}
		}()
		go func() {
			defer wg.Done()

			for range 1000 {
				_, _ = getter.Get(tokenKey)
			}
		}()

		wg.Wait()
	})

	It("opens the same store from a handle that travelled through the dependency map", func() {
		store := secrets.NewStore()
		communicator := open(store.Handle(), communicatorPath)
		Expect(communicator.Put(tokenKey, secrets.NewValue("v1"))).To(Succeed())

		root := map[string]any{}
		config.SetDependency(root, secrets.StoreKey, store.Handle())

		child := map[string]any{"other": 1}
		merged := config.MergeDependencies(root, child)

		handle, ok := config.LookupDependency(merged, secrets.StoreKey)
		Expect(ok).To(BeTrue())

		scope := open(handle, communicatorPath)
		value, err := scope.Get(tokenKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(value.Reveal()).To(Equal("v1"))
	})

	It("checks that JSON and fmt output of the handle, the dependency map and a ChildSpec do not contain a stored value", func() {
		store := secrets.NewStore()
		scope := open(store.Handle(), communicatorPath)
		Expect(scope.Put(tokenKey, secrets.NewValue(planted))).To(Succeed())

		handle := store.Handle()
		handlePointer := &handle
		dependencyMap := map[string]any{}
		config.SetDependency(dependencyMap, secrets.StoreKey, handle)
		spec := config.ChildSpec{Dependencies: dependencyMap}

		objects := []struct {
			name  string
			value any
		}{
			{"handle", handle},
			{"&handle", handlePointer},
			{"dependency map", dependencyMap},
			{"ChildSpec", spec},
		}
		for _, object := range objects {
			encoded, err := json.Marshal(object.value)
			Expect(err).NotTo(HaveOccurred(), "json.Marshal of "+object.name)
			Expect(string(encoded)).NotTo(ContainSubstring(planted), "json.Marshal of "+object.name)

			for verb, out := range map[string]string{
				"%v":  fmt.Sprintf("%v", object.value),
				"%+v": fmt.Sprintf("%+v", object.value),
				"%#v": fmt.Sprintf("%#v", object.value),
				"%s":  fmt.Sprintf("%s", object.value),
			} {
				Expect(out).NotTo(ContainSubstring(planted), verb+" of "+object.name)
			}
		}

		// Positive control: the same renderings of a plain string field do
		// contain the value.
		probe := struct{ S string }{planted}
		encoded, err := json.Marshal(probe)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).To(ContainSubstring(planted))
		Expect(fmt.Sprintf("%v", probe)).To(ContainSubstring(planted))
		Expect(fmt.Sprintf("%+v", probe)).To(ContainSubstring(planted))
		Expect(fmt.Sprintf("%#v", probe)).To(ContainSubstring(planted))
		Expect(fmt.Sprintf("%s", probe)).To(ContainSubstring(planted))
	})
})

var _ = Describe("NewKey", func() {
	It("panics on an empty name, a name with \"/\", \".\" and \"..\"", func() {
		for _, name := range []string{"", "a/b", ".", ".."} {
			Expect(func() { secrets.NewKey(name) }).To(Panic(), "NewKey(%q)", name)
		}
	})

	It("accepts a plain name, and the key it returns stores and reads an entry", func() {
		var key secrets.Key

		Expect(func() { key = secrets.NewKey("transport.jwt") }).NotTo(Panic())

		scope := open(secrets.NewStore().Handle(), transportPath)
		Expect(scope.Put(key, secrets.NewValue("v1"))).To(Succeed())
		value, err := scope.Get(key)
		Expect(err).NotTo(HaveOccurred())
		Expect(value.Reveal()).To(Equal("v1"))
	})
})
