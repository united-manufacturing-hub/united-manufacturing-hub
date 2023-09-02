package internal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func basicTestHttpServer(gs GracefulShutdownHandler) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if gs.ShuttingDown() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		gs.Shutdown()
		w.WriteHeader(http.StatusOK)
	})

	return &http.Server{Addr: ":8080", Handler: mux}
}

func Test_NewGracefulShutdown(t *testing.T) {
	var gs GracefulShutdownHandler
	var srv *http.Server

	// The passed function only runs after a SIGTERM/SIGINT signal is received.
	gs = NewGracefulShutdown(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := srv.Shutdown(ctx)
		return err
	})
	defer gs.Wait() // Wait for shutdown tasks to complete before exiting.

	// Create a basic http server and start listening for requests.
	srv = basicTestHttpServer(gs)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Error starting server: %s", err)
		}
	}()

	// Order of requests is important.
	tcs := []struct {
		url                string
		expectedStatusCode int
	}{
		{"http://localhost:8080/health", http.StatusOK},                 // Server is up during initial request.
		{"http://localhost:8080/shutdown", http.StatusOK},               // Request to /shutdown calls gs.Shutdown()
		{"http://localhost:8080/health", http.StatusServiceUnavailable}, // After shutdown request, a 503 is expected.
	}

	for _, tc := range tcs {
		name := fmt.Sprintf("test request %s", tc.url)
		t.Run(name, func(t *testing.T) {
			res, err := http.Get(tc.url)
			if err != nil {
				t.Errorf("Error sending GET request to %s: %s", tc.url, err)
			}

			if res.StatusCode != tc.expectedStatusCode {
				t.Errorf("Expected status code for %s to be %d, got %d", tc.url, tc.expectedStatusCode, res.StatusCode)
			}
		})
	}
}
