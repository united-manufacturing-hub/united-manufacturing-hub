package cse

import (
	"errors"
	"fmt"
	"sync"
)

type Closeable interface {
	Close() error
}

type Factory func() (interface{}, error)

type ObjectPool struct {
	objects map[string]interface{}
	refs    map[string]int
	mu      sync.RWMutex
}

func NewObjectPool() *ObjectPool {
	return &ObjectPool{
		objects: make(map[string]interface{}),
		refs:    make(map[string]int),
	}
}

func (p *ObjectPool) Get(key string) (interface{}, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	obj, found := p.objects[key]

	return obj, found
}

func (p *ObjectPool) Put(key string, obj interface{}) error {
	if key == "" {
		return errors.New("cannot put object with empty key")
	}

	if obj == nil {
		return errors.New("cannot put nil object")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.objects[key] = obj
	p.refs[key] = 1

	return nil
}

func (p *ObjectPool) Remove(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	obj, found := p.objects[key]
	if !found {
		return nil
	}

	var closeErr error
	if closeable, ok := obj.(Closeable); ok {
		closeErr = closeable.Close()
	}

	delete(p.objects, key)
	delete(p.refs, key)

	return closeErr
}

func (p *ObjectPool) Has(key string) bool {
	if key == "" {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	_, found := p.objects[key]

	return found
}

func (p *ObjectPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.objects)
}

func (p *ObjectPool) Clear() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error

	for key, obj := range p.objects {
		if closeable, ok := obj.(Closeable); ok {
			if err := closeable.Close(); err != nil {
				errs = append(errs, fmt.Errorf("error closing %s: %w", key, err))
			}
		}
	}

	p.objects = make(map[string]interface{})
	p.refs = make(map[string]int)

	if len(errs) > 0 {
		return fmt.Errorf("errors during clear: %v", errs)
	}

	return nil
}

func (p *ObjectPool) GetOrCreate(key string, factory Factory) (interface{}, error) {
	if key == "" {
		return nil, errors.New("cannot get or create object with empty key")
	}

	if factory == nil {
		return nil, errors.New("nil factory provided")
	}

	p.mu.RLock()
	obj, found := p.objects[key]
	p.mu.RUnlock()

	if found {
		return obj, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	obj, found = p.objects[key]
	if found {
		return obj, nil
	}

	newObj, err := factory()
	if err != nil {
		return nil, err
	}

	if newObj == nil {
		return nil, errors.New("factory returned nil object")
	}

	p.objects[key] = newObj
	p.refs[key] = 1

	return newObj, nil
}

func (p *ObjectPool) Acquire(key string) error {
	if key == "" {
		return errors.New("cannot acquire object with empty key")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, found := p.objects[key]; !found {
		return fmt.Errorf("object with key %s not found", key)
	}

	p.refs[key]++

	return nil
}

func (p *ObjectPool) Release(key string) error {
	if key == "" {
		return errors.New("cannot release object with empty key")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	obj, found := p.objects[key]
	if !found {
		return fmt.Errorf("object with key %s not found", key)
	}

	p.refs[key]--

	if p.refs[key] <= 0 {
		var closeErr error
		if closeable, ok := obj.(Closeable); ok {
			closeErr = closeable.Close()
		}

		delete(p.objects, key)
		delete(p.refs, key)

		return closeErr
	}

	return nil
}
