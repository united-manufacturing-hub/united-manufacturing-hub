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


package fsmv2

// BaseWorker provides dependencies access to all workers.
// Workers should embed this struct to minimize boilerplate.
//
// Example usage:
//
//	type MyWorker struct {
//	    *fsmv2.BaseWorker[*MyDependencies]
//	    // other worker-specific fields
//	}
//
//	func NewMyWorker(dependencies *MyDependencies) *MyWorker {
//	    return &MyWorker{
//	        BaseWorker: fsmv2.NewBaseWorker(dependencies),
//	    }
//	}
type BaseWorker[D Dependencies] struct {
	dependencies D
}

// NewBaseWorker creates a new BaseWorker with the given dependencies.
func NewBaseWorker[D Dependencies](dependencies D) *BaseWorker[D] {
	return &BaseWorker[D]{dependencies: dependencies}
}

// GetDependencies returns the dependencies for this worker.
func (w *BaseWorker[D]) GetDependencies() D {
	return w.dependencies
}
