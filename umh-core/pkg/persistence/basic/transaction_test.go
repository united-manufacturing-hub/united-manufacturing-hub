package basic_test

import (
	"context"
	"errors"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

type mockStore struct {
	beginTxCalled int
	beginTxErr    error
	txCommitted   bool
	txRolledBack  bool
}

func (m *mockStore) BeginTx(ctx context.Context) (basic.Tx, error) {
	m.beginTxCalled++
	if m.beginTxErr != nil {
		return nil, m.beginTxErr
	}
	return &mockTx{store: m}, nil
}

func (m *mockStore) CreateCollection(ctx context.Context, name string, schema *basic.Schema) error {
	panic("not implemented")
}

func (m *mockStore) DropCollection(ctx context.Context, name string) error {
	panic("not implemented")
}

func (m *mockStore) Insert(ctx context.Context, collection string, doc basic.Document) (string, error) {
	panic("not implemented")
}

func (m *mockStore) Get(ctx context.Context, collection string, id string) (basic.Document, error) {
	panic("not implemented")
}

func (m *mockStore) Update(ctx context.Context, collection string, id string, doc basic.Document) error {
	panic("not implemented")
}

func (m *mockStore) Delete(ctx context.Context, collection string, id string) error {
	panic("not implemented")
}

func (m *mockStore) Find(ctx context.Context, collection string, query basic.Query) ([]basic.Document, error) {
	panic("not implemented")
}

func (m *mockStore) Close() error {
	panic("not implemented")
}

type mockTx struct {
	store *mockStore
}

func (t *mockTx) Commit() error {
	t.store.txCommitted = true
	return nil
}

func (t *mockTx) Rollback() error {
	t.store.txRolledBack = true
	return nil
}

func (t *mockTx) BeginTx(ctx context.Context) (basic.Tx, error) {
	panic("not implemented")
}

func (t *mockTx) CreateCollection(ctx context.Context, name string, schema *basic.Schema) error {
	return nil
}

func (t *mockTx) DropCollection(ctx context.Context, name string) error {
	return nil
}

func (t *mockTx) Insert(ctx context.Context, collection string, doc basic.Document) (string, error) {
	return "", nil
}

func (t *mockTx) Get(ctx context.Context, collection string, id string) (basic.Document, error) {
	return nil, nil
}

func (t *mockTx) Update(ctx context.Context, collection string, id string, doc basic.Document) error {
	return nil
}

func (t *mockTx) Delete(ctx context.Context, collection string, id string) error {
	return nil
}

func (t *mockTx) Find(ctx context.Context, collection string, query basic.Query) ([]basic.Document, error) {
	return nil, nil
}

func (t *mockTx) Close() error {
	panic("not implemented")
}

func TestWithTransaction_Success(t *testing.T) {
	store := &mockStore{}

	err := basic.WithTransaction(context.Background(), store, func(tx basic.Tx) error {
		return nil
	})
	if err != nil {
		t.Errorf("WithTransaction() returned error: %v", err)
	}

	if !store.txCommitted {
		t.Error("Transaction was not committed")
	}

	if store.txRolledBack {
		t.Error("Transaction was rolled back (should have been committed)")
	}
}

func TestWithTransaction_Error(t *testing.T) {
	store := &mockStore{}
	expectedErr := errors.New("transaction failed")
	err := basic.WithTransaction(context.Background(), store, func(tx basic.Tx) error {
		return expectedErr
	})

	if !errors.Is(err, expectedErr) && err.Error() != expectedErr.Error() {
		t.Errorf("WithTransaction() error = %v, want %v", err, expectedErr)
	}

	if store.txCommitted {
		t.Error("Transaction was committed (should have been rolled back)")
	}

	if !store.txRolledBack {
		t.Error("Transaction was not rolled back")
	}
}

func TestWithTransaction_Panic(t *testing.T) {
	store := &mockStore{}

	defer func() {
		if r := recover(); r == nil {
			t.Error("WithTransaction() did not re-panic")
		}

		if !store.txRolledBack {
			t.Error("Transaction was not rolled back after panic")
		}

		if store.txCommitted {
			t.Error("Transaction was committed after panic")
		}
	}()

	_ = basic.WithTransaction(context.Background(), store, func(tx basic.Tx) error {
		panic("something went wrong")
	})
}

func TestWithTransaction_ContextCancelled(t *testing.T) {
	store := &mockStore{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := basic.WithTransaction(ctx, store, func(tx basic.Tx) error {
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("WithTransaction() error = %v, want %v", err, context.Canceled)
	}

	if store.txCommitted {
		t.Error("Transaction was committed (should have been rolled back)")
	}
}

func TestWithRetry_Success(t *testing.T) {
	store := &mockStore{}
	callCount := 0

	err := basic.WithRetry(context.Background(), store, 3, func(tx basic.Tx) error {
		callCount++
		return nil
	})
	if err != nil {
		t.Errorf("WithRetry() returned error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Function called %d times, expected 1", callCount)
	}
}

func TestWithRetry_Conflict(t *testing.T) {
	store := &mockStore{}
	callCount := 0

	err := basic.WithRetry(context.Background(), store, 3, func(tx basic.Tx) error {
		callCount++
		if callCount < 3 {
			return basic.ErrConflict
		}
		return nil
	})
	if err != nil {
		t.Errorf("WithRetry() returned error: %v", err)
	}

	if callCount != 3 {
		t.Errorf("Function called %d times, expected 3", callCount)
	}
}

func TestWithRetry_ExceedsMaxRetries(t *testing.T) {
	store := &mockStore{}
	callCount := 0

	err := basic.WithRetry(context.Background(), store, 3, func(tx basic.Tx) error {
		callCount++
		return basic.ErrConflict
	})

	if !errors.Is(err, basic.ErrConflict) {
		t.Errorf("WithRetry() error = %v, want ErrConflict", err)
	}

	if callCount != 4 {
		t.Errorf("Function called %d times, expected 4", callCount)
	}
}

func TestWithRetry_NonConflictError(t *testing.T) {
	store := &mockStore{}
	callCount := 0
	otherErr := errors.New("other error")

	err := basic.WithRetry(context.Background(), store, 3, func(tx basic.Tx) error {
		callCount++
		return otherErr
	})

	if !errors.Is(err, otherErr) && err.Error() != otherErr.Error() {
		t.Errorf("WithRetry() error = %v, want %v", err, otherErr)
	}

	if callCount != 1 {
		t.Errorf("Function called %d times, expected 1 (no retry for non-conflict)", callCount)
	}
}

func TestWithRetry_NegativeMaxRetries(t *testing.T) {
	store := &mockStore{}

	err := basic.WithRetry(context.Background(), store, -1, func(tx basic.Tx) error {
		return nil
	})
	if err == nil {
		t.Error("WithRetry() should return error for negative maxRetries")
	}

	if store.beginTxCalled > 0 {
		t.Error("BeginTx should not be called with negative maxRetries")
	}
}
