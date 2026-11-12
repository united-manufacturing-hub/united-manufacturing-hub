// Copyright 2026 UMH Systems GmbH
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

package main

import (
	"fmt"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/persistence/memory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	persistenceWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
)

// expectedWorkerTypes lists the worker types the walk below must reach.
// The walk fails when it reaches fewer of them, so a render that drops a child
// cannot shrink the set that the registry checks run on. The connection
// workers come from the Management Console module and are checked there.
var expectedWorkerTypes = []string{"persistence", "application"}

// extraWorkerTypes lists worker types that umh-core starts but that no render
// function below returns. The application adds configworker as its kernel
// child. The config worker adds cpu and historian-timescale, and the
// connection service adds nmap. These types are checked like the walked ones,
// but they do not count towards expectedWorkerTypes.
var extraWorkerTypes = []string{"configworker", "cpu", "historian-timescale", "nmap"}

var _ = Describe("production worker tree", func() {
	// A worker type can start only if two init functions ran in this binary.
	// register.Worker adds it to the factory, which ValidateChildSpec reads and
	// NewWorkerByType builds from. Most state packages call
	// fsmv2.RegisterInitialState, which WorkerBase.GetInitialState reads. Both
	// run only when main links the package that calls them. This test builds
	// the child specs with the same render function main calls. For every
	// worker type it finds, it then does what a supervisor does for a child: it
	// creates the worker through the factory and asks it for its initial state.
	It("creates every worker type it renders through the factory and reads its initial state", func() {
		var walked []string
		reached := map[string]bool{}
		reach := func(workerType string) {
			if !reached[workerType] {
				reached[workerType] = true
				walked = append(walked, workerType)
			}
		}

		// Root level: the children main gives the application supervisor when
		// nothing is registered.
		doc, err := renderSupervisorChildrenYAML(nil)
		Expect(err).NotTo(HaveOccurred())

		var root struct {
			Children []struct {
				WorkerType string `yaml:"workerType"`
			} `yaml:"children"`
		}
		Expect(yaml.Unmarshal([]byte(doc), &root)).To(Succeed())
		for _, child := range root.Children {
			reach(child.WorkerType)
		}

		// The root is the application, which no render returns.
		found := map[string]bool{"application": true}
		for _, workerType := range walked {
			found[workerType] = true
		}

		var missing []string
		for _, workerType := range expectedWorkerTypes {
			if !found[workerType] {
				missing = append(missing, workerType)
			}
		}
		Expect(missing).To(BeEmpty(), "the walk did not reach these worker types: %s", strings.Join(missing, ", "))

		// Parents come before their children, because the transport worker
		// publishes the dependencies its push and pull children are built from.
		checked := append([]string{"application"}, walked...)
		for _, workerType := range extraWorkerTypes {
			if !found[workerType] {
				found[workerType] = true
				checked = append(checked, workerType)
			}
		}

		expected := map[string]bool{}
		for _, workerType := range expectedWorkerTypes {
			expected[workerType] = true
		}

		var extra []string
		for _, workerType := range checked {
			if !expected[workerType] {
				extra = append(extra, workerType)
			}
		}
		sort.Strings(extra)
		GinkgoWriter.Printf("worker types checked beyond the fixed list: %s\n", strings.Join(extra, ", "))

		store, dependencies := newStandInDependencies()

		registered := map[string]bool{}
		for _, workerType := range factory.ListRegisteredTypes() {
			registered[workerType] = true
		}

		var failures []string
		for _, workerType := range checked {
			failures = append(failures, checkWorkerType(workerType, registered, store, dependencies)...)
		}
		Expect(failures).To(BeEmpty(), "umh-core renders these worker types but cannot start them:\n%s", strings.Join(failures, "\n"))
	})
})

// newStandInDependencies returns a store and a dependency map that holds the
// keys main sets for the workers this test creates, with stand-in values. It
// also publishes the configworker registry, as main does. The spec's cleanup puts
// back the values that were published before it ran.
func newStandInDependencies() (storage.TriangularStoreInterface, map[string]any) {
	keepGlobalDeps[*dynamicchildren.Registry](configworker.WorkerTypeName)
	register.SetGlobalDeps(configworker.WorkerTypeName, dynamicchildren.NewWriter().Registry())

	var store storage.TriangularStoreInterface = storage.NewTriangularStore(memory.NewInMemoryStore(), deps.NewNopFSMLogger())

	dependencies := map[string]any{}
	fsmv2config.SetDependency(dependencies, persistenceWorker.StoreKey, store)

	return store, dependencies
}

// keepGlobalDeps makes the spec's cleanup put back the value published under
// key when it was called, or remove the key when nothing was published.
func keepGlobalDeps[T comparable](key string) {
	previous := register.GlobalDeps[T](key)
	DeferCleanup(func() {
		var none T
		if previous == none {
			register.ClearGlobalDeps(key)

			return
		}

		register.SetGlobalDeps(key, previous)
	})
}

// checkWorkerType runs the two checks on workerType and returns one line per
// failed check, each naming the type.
// (a) The factory lists the type. ValidateChildSpec checks the same list.
// (b) factory.NewWorkerByType creates the worker, and GetInitialState returns a
// state without panicking. A supervisor makes the same two calls for a child.
func checkWorkerType(workerType string, registered map[string]bool, store storage.TriangularStoreInterface, dependencies map[string]any) []string {
	var failures []string
	if !registered[workerType] {
		failures = append(failures, fmt.Sprintf("worker type %s: (a) not in factory.ListRegisteredTypes", workerType))
	}

	identity := deps.Identity{
		ID:            workerType + "-001",
		Name:          workerType,
		WorkerType:    workerType,
		HierarchyPath: workerType + "-001(" + workerType + ")",
	}

	worker, err := withRecover(func() (fsmv2.Worker, error) {
		return factory.NewWorkerByType(workerType, identity, deps.NewNopFSMLogger(), store, dependencies)
	})
	if err != nil {
		return append(failures, fmt.Sprintf("worker type %s: (b) factory.NewWorkerByType: %v", workerType, err))
	}

	if worker == nil {
		return append(failures, fmt.Sprintf("worker type %s: (b) factory.NewWorkerByType returned no worker", workerType))
	}

	initial, err := withRecover(func() (fsmv2.State[any, any], error) {
		return worker.GetInitialState(), nil
	})
	if err != nil {
		return append(failures, fmt.Sprintf("worker type %s: (b) GetInitialState: %v", workerType, err))
	}

	if initial == nil {
		failures = append(failures, fmt.Sprintf("worker type %s: (b) GetInitialState returned nil", workerType))
	}

	return failures
}

// withRecover calls f and returns a panic in f as an error that holds the panic text.
func withRecover[T any](f func() (T, error)) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panicked: %v", r)
		}
	}()

	return f()
}
