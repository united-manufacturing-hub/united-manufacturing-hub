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
