package filesystem

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/metrics"
)

// Service provides an interface for filesystem operations
// This allows for easier testing and separation of concerns
type Service interface {
	// EnsureDirectory creates a directory if it doesn't exist
	EnsureDirectory(ctx context.Context, path string) error

	// ReadFile reads a file's contents respecting the context
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// WriteFile writes data to a file respecting the context
	WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error

	// FileExists checks if a file exists
	FileExists(ctx context.Context, path string) (bool, error)

	// Remove removes a file or directory
	Remove(ctx context.Context, path string) error

	// RemoveAll removes a directory and all its contents
	RemoveAll(ctx context.Context, path string) error

	// MkdirTemp creates a new temporary directory
	MkdirTemp(ctx context.Context, dir, pattern string) (string, error)

	// Stat returns file info
	Stat(ctx context.Context, path string) (os.FileInfo, error)

	// CreateFile creates a new file with the specified permissions
	CreateFile(ctx context.Context, path string, perm os.FileMode) (*os.File, error)

	// Chmod changes the mode of the named file
	Chmod(ctx context.Context, path string, mode os.FileMode) error

	// ReadDir reads a directory, returning all its directory entries
	ReadDir(ctx context.Context, path string) ([]os.DirEntry, error)

	// ExecuteCommand executes a command with context
	ExecuteCommand(ctx context.Context, name string, args ...string) ([]byte, error)
}

// DefaultService is the default implementation of FileSystemService
type DefaultService struct{}

// NewDefaultService creates a new DefaultFileSystemService
func NewDefaultService() *DefaultService {
	return &DefaultService{}
}

// checkContext checks if the context is done before proceeding with an operation
func (s *DefaultService) checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// EnsureDirectory creates a directory if it doesn't exist
func (s *DefaultService) EnsureDirectory(ctx context.Context, path string) error {
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run operation in goroutine
	go func() {
		errCh <- os.MkdirAll(path, 0755)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReadFile reads a file's contents respecting the context
func (s *DefaultService) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		data []byte
		err  error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		data, err := os.ReadFile(path)
		resCh <- result{data, err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, res.err
		}
		return res.data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WriteFile writes data to a file respecting the context
func (s *DefaultService) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.WriteFile(path, data, perm)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", path, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FileExists checks if a file exists
func (s *DefaultService) FileExists(ctx context.Context, path string) (bool, error) {
	if err := s.checkContext(ctx); err != nil {
		return false, err
	}

	// Create a channel for results
	type result struct {
		exists bool
		err    error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			resCh <- result{false, nil}
			return
		}
		if err != nil {
			resCh <- result{false, fmt.Errorf("failed to check if file exists: %w", err)}
			return
		}
		resCh <- result{true, nil}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return false, res.err
		}
		return res.exists, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// Remove removes a file or directory
func (s *DefaultService) Remove(ctx context.Context, path string) error {
	if err := s.checkContext(ctx); err != nil {
		return err
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.Remove(path)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RemoveAll removes a directory and all its contents
func (s *DefaultService) RemoveAll(ctx context.Context, path string) error {
	if err := s.checkContext(ctx); err != nil {
		return err
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.RemoveAll(path)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to remove directory %s: %w", path, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// MkdirTemp creates a new temporary directory
func (s *DefaultService) MkdirTemp(ctx context.Context, dir, pattern string) (string, error) {
	if err := s.checkContext(ctx); err != nil {
		return "", fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		path string
		err  error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		path, err := os.MkdirTemp(dir, pattern)
		resCh <- result{path, err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return "", fmt.Errorf("failed to create temporary directory: %w", res.err)
		}
		return res.path, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Stat returns file info
func (s *DefaultService) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		info os.FileInfo
		err  error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		info, err := os.Stat(path)
		resCh <- result{info, err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("failed to get file info: %w", res.err)
		}
		return res.info, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CreateFile creates a new file with the specified permissions
func (s *DefaultService) CreateFile(ctx context.Context, path string, perm os.FileMode) (*os.File, error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		file *os.File
		err  error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
		resCh <- result{file, err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("failed to create file: %w", res.err)
		}
		return res.file, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Chmod changes the mode of the named file
func (s *DefaultService) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	if err := s.checkContext(ctx); err != nil {
		return fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	errCh := make(chan error, 1)

	// Run file operation in goroutine
	go func() {
		errCh <- os.Chmod(path, mode)
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to change mode of file %s: %w", path, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReadDir reads a directory, returning all its directory entries
func (s *DefaultService) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// Create a channel for results
	type result struct {
		entries []os.DirEntry
		err     error
	}
	resCh := make(chan result, 1)

	// Run file operation in goroutine
	go func() {
		entries, err := os.ReadDir(path)
		resCh <- result{entries, err}
	}()

	// Wait for either completion or context cancellation
	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("failed to read directory %s: %w", path, res.err)
		}
		return res.entries, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ExecuteCommand executes a command with context
func (s *DefaultService) ExecuteCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentFilesystem, name+".executeCommand."+name, time.Since(start))
	}()

	if err := s.checkContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to check context: %w", err)
	}

	// This method already respects context cancellation through exec.CommandContext
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute command %s: %w", name, err)
	}
	return output, nil
}
