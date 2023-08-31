package internal

import (
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

type GracefulShutdownHandler interface {
	Shutdown()          // Trigger a graceful shutdown programmatically.
	ShuttingDown() bool // Quickly check if a shutdown is in progress.
}

// A channel that waits/reads for a SIGTERM signal.
type sigChan chan os.Signal

// Initialize a graceful shutdown handler.
// Takes a function that runs after a SIGTERM signal is received (if not nil).
func NewGracefulShutdown(onShutdown func() error) GracefulShutdownHandler {
	sigs := make(sigChan, 1)

	go func(sigs chan os.Signal) {
		signal.Notify(sigs, syscall.SIGTERM)
		// before you trapped SIGTERM your process would
		// have exited, so we are now on borrowed time.
		//
		// Kubernetes sends SIGTERM 30 seconds before
		// shutting down the pod.
		sig := <-sigs
		zap.S().Infof("Received SIGTERM (%s), shutting down", sig)
		if onShutdown != nil {
			err := onShutdown()
			if err != nil {
				zap.S().Fatalf("Error during shutdown: %s", err)
				return
			}
		}
		zap.S().Info("Successful shutdown. Exiting.")
		os.Exit(0)
	}(sigs)

	return sigs
}

func (s sigChan) ShuttingDown() bool {
	select {
	case <-s:
		return true
	default:
		return false
	}
}

func (s sigChan) Shutdown() {
	s <- syscall.SIGTERM
}
