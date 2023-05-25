// Copyright 2023 UMH Systems GmbH
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

package main

import (
	"fmt"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"go.uber.org/zap"
	"math/rand"
	"net/http"
	"os"
	"time"
)

func main() {
	// Initialize zap logging
	logLevel, _ := env.GetAsString("LOGGING_LEVEL", false, "PRODUCTION") //nolint:errcheck
	log := logger.New(logLevel)
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	// Print all env variables
	zap.S().Debugf("Printing all env variables")
	for _, v := range os.Environ() {
		zap.S().Debugf("%s", v)
	}

	// Write to stateful storage
	zap.S().Debugf("Writing to stateful storage")

	// Check if hello-world file exists
	if _, err := os.Stat("/data/hello-world"); os.IsNotExist(err) {
		zap.S().Debugf("hello-world file does not exist")
		// Create hello-world file
		zap.S().Debugf("Creating hello-world file")
		err := os.WriteFile("/data/hello-world", []byte("Hello World"), 0600)
		if err != nil {
			zap.S().Fatalf("Failed to create hello-world file: %s", err)
		}
	} else {
		zap.S().Debugf("hello-world file exists")
	}

	go sampleWebServer()
	go livenessServer()

	// Wait forever
	select {}
}

var bg = New()

// Returns random liveness status
func livenessHandler(w http.ResponseWriter, _ *http.Request) {
	if bg.Bool() {
		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprintf(w, "OK")
		if err != nil {
			zap.S().Errorf("Error writing response: %s", err)
		}
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := fmt.Fprintf(w, "Internal Server Error")
		if err != nil {
			zap.S().Errorf("Error writing response: %s", err)
		}
	}
}

func livenessServer() {
	serverMuxA := http.NewServeMux()
	serverMuxA.HandleFunc("/health", livenessHandler)
	/* #nosec G114 */
	/* #nosec G114 */
	err := http.ListenAndServe(":9091", serverMuxA)
	if err != nil {
		zap.S().Fatalf("Error starting web server: %s", err)
	}
}

func sampleWebServer() {
	serverMuxB := http.NewServeMux()
	serverMuxB.HandleFunc("/health", livenessHandler)
	/* #nosec G114 */
	/* #nosec G114 */
	err := http.ListenAndServe(":81", serverMuxB)
	if err != nil {
		zap.S().Fatalf("Error starting web server: %s", err)
	}
}

// Random bool generation (https://stackoverflow.com/questions/45030618/generate-a-random-bool-in-go)

type Boolgen struct {
	src       rand.Source
	cache     int64
	remaining int
}

func (b *Boolgen) Bool() bool {
	if b.remaining == 0 {
		b.cache, b.remaining = b.src.Int63(), 63
	}

	result := b.cache&0x01 == 1
	b.cache >>= 1
	b.remaining--

	return result
}

func New() *Boolgen {
	return &Boolgen{src: rand.NewSource(time.Now().UnixNano())}
}
