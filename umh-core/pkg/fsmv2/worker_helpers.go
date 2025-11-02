package fsmv2

// BaseWorker provides registry access to all workers.
// Workers should embed this struct to minimize boilerplate.
//
// Example usage:
//
//	type MyWorker struct {
//	    *fsmv2.BaseWorker[*MyRegistry]
//	    // other worker-specific fields
//	}
//
//	func NewMyWorker(registry *MyRegistry) *MyWorker {
//	    return &MyWorker{
//	        BaseWorker: fsmv2.NewBaseWorker(registry),
//	    }
//	}
type BaseWorker[R Registry] struct {
	registry R
}

// NewBaseWorker creates a new BaseWorker with the given registry.
func NewBaseWorker[R Registry](registry R) *BaseWorker[R] {
	return &BaseWorker[R]{registry: registry}
}

// GetRegistry returns the registry for this worker.
func (w *BaseWorker[R]) GetRegistry() R {
	return w.registry
}
