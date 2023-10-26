package internal

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

type GracefulShutdownHandler interface {
	Shutdown()          // Triggers a graceful shutdown programmatically.
	ShuttingDown() bool // Quickly checks if a shutdown is in progress.
	Wait()              // Blocks until shutdown tasks are complete.
}

type gracefulShutdown struct {
	quit         chan os.Signal // Blocks until a SIGTERM/SIGINT signal is received.
	shuttingDown chan bool      // Indicates if a shutdown is happening.
	wg           sync.WaitGroup // Waits until all shutdown tasks are complete.
}

// Initialize a graceful shutdown handler.
// Takes a function that runs after a SIGTERM/SIGINT signal is received (if not nil).
func NewGracefulShutdown(onShutdown func() error) GracefulShutdownHandler {
	gs := &gracefulShutdown{
		quit:         make(chan os.Signal, 1),
		shuttingDown: make(chan bool, 1),
		wg:           sync.WaitGroup{},
	}
	gs.wg.Add(1)

	go func(gs *gracefulShutdown, onShutdown func() error) {
		defer gs.wg.Done()
		signal.Notify(gs.quit, syscall.SIGINT, syscall.SIGTERM)
		// before you trapped SIGTERM your process would
		// have exited, so we are now on borrowed time.
		//
		// Kubernetes sends SIGTERM 30 seconds before
		// shutting down the pod.
		sig := <-gs.quit
		gs.shuttingDown <- true
		zap.S().Infow("Received signal, shutting down", "signal", sig.String())
		if onShutdown != nil {
			timeout := 30 * time.Second
			zap.S().Infow("Waiting for shutdown tasks to complete", "timeout", timeout)
			go func(t time.Duration) {
				select {
				case <-time.After(t):
					zap.S().Errorw("Shutdown tasks did not complete in time", "timeout", t)
					// Flush buffer
					_ = zap.S().Sync()
					os.Exit(1)
				}
			}(timeout)
			err := onShutdown()
			if err != nil {
				zap.S().Errorw("Error during shutdown", "error", err)
				return
			}
		}
		zap.S().Info("Shutdown tasks completed. Ready to exit.")
		os.Exit(0)
	}(gs, onShutdown)

	return gs
}

func (gs *gracefulShutdown) ShuttingDown() bool {
	select {
	case <-gs.shuttingDown:
		// Put the value back, in case it's checked again later during shutdown.
		gs.shuttingDown <- true
		return true
	default:
		return false
	}
}

func (gs *gracefulShutdown) Shutdown() {
	// Only send a SIGTERM signal if we are not already shutting down.
	if !gs.ShuttingDown() {
		gs.quit <- syscall.SIGTERM
	}
}

func (gs *gracefulShutdown) Wait() {
	gs.wg.Wait()
}
