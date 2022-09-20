package main

import (
	"fmt"
	"github.com/united-manufacturing-hub/umh-utils/logger"
	"go.uber.org/zap"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"
)

var buildtime string

func main() {
	// Initialize zap logging
	log := logger.New("LOGGING_LEVEL")
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(log)

	zap.S().Infof("This is custom-microservice-tester build date: %s", buildtime)

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
		err := ioutil.WriteFile("/data/hello-world", []byte("Hello World"), 0644)
		if err != nil {
			panic(err)
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
func livenessHandler(w http.ResponseWriter, req *http.Request) {
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
	err := http.ListenAndServe(":9091", serverMuxA)
	if err != nil {
		zap.S().Fatalf("Error starting web server: %s", err)
	}
}

func sampleWebServer() {
	serverMuxB := http.NewServeMux()
	serverMuxB.HandleFunc("/health", livenessHandler)
	err := http.ListenAndServe(":81", serverMuxB)
	if err != nil {
		zap.S().Fatalf("Error starting web server: %s", err)
	}
}

// Random bool generation (https://stackoverflow.com/questions/45030618/generate-a-random-bool-in-go)

type boolgen struct {
	src       rand.Source
	cache     int64
	remaining int
}

func (b *boolgen) Bool() bool {
	if b.remaining == 0 {
		b.cache, b.remaining = b.src.Int63(), 63
	}

	result := b.cache&0x01 == 1
	b.cache >>= 1
	b.remaining--

	return result
}

func New() *boolgen {
	return &boolgen{src: rand.NewSource(time.Now().UnixNano())}
}
