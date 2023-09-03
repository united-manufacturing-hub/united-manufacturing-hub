package internal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func httptestBasicServer(gs GracefulShutdownHandler) *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if gs.ShuttingDown() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		// Triggers the execution of the onShutdown passed to NewGracefulShutdown.
		gs.Shutdown()
		w.WriteHeader(http.StatusOK)
	})

	return httptest.NewServer(mux)
}

func Test_NewGracefulShutdown(t *testing.T) {
	var reqWg sync.WaitGroup // To wait for all requests to complete before closing the server.
	var testSrv *httptest.Server

	// Only close the httptest server after a /shutdown request is made,
	// which initiates the graceful shutdown.
	gs := NewGracefulShutdown(func() error {
		reqWg.Wait()
		testSrv.Close()
		return nil
	})
	defer gs.Wait() // Wait for shutdown tasks to complete before exiting.

	// Create a basic httptest server and start listening for requests.
	testSrv = httptestBasicServer(gs)
	healthRoute := fmt.Sprintf("%s/health", testSrv.URL)
	shutdownRoute := fmt.Sprintf("%s/shutdown", testSrv.URL)

	// Order of requests is important.
	tcs := []struct {
		url                string
		expectedStatusCode int
	}{
		{healthRoute, http.StatusOK},                 // Server is up during initial request.
		{shutdownRoute, http.StatusOK},               // Request to /shutdown calls gs.Shutdown()
		{healthRoute, http.StatusServiceUnavailable}, // After shutdown request, a 503 is expected.
	}

	reqWg.Add(len(tcs))
	for _, tc := range tcs {
		name := fmt.Sprintf("test request %s", tc.url)
		t.Run(name, func(t *testing.T) {
			defer reqWg.Done()

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
