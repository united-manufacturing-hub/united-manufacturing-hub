package main

import (
	"fmt"
	"go.elastic.co/ecszap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"
)

var buildtime string

func main() {
	// Initialize zap logging
	encoderConfig := ecszap.NewDefaultEncoderConfig()
	var core zapcore.Core
	core = ecszap.NewCore(encoderConfig, os.Stdout, zap.DebugLevel)
	logger := zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(logger)
	defer func(logger *zap.Logger) {
		err := logger.Sync()
		if err != nil {
			panic(err)
		}
	}(logger)

	zap.S().Infof("This is custom-microservice-tester build date: %s", buildtime)

	// pprof for port testing
	zap.S().Debugf("Starting pprof server on port 8080")
	go http.ListenAndServe(":8080", nil)

	// Print all env variables
	zap.S().Debugf("Printing all env variables")
	for _, v := range os.Environ() {
		zap.S().Debugf("%s", v)
	}

	// Write to stateful storage
	zap.S().Debugf("Writing to stateful storage")

	// Check if hello-world file exists
	if _, err := os.Stat("/tmp/hello-world"); os.IsNotExist(err) {
		zap.S().Debugf("hello-world file does not exist")
		// Create hello-world file
		zap.S().Debugf("Creating hello-world file")
		err := ioutil.WriteFile("/tmp/hello-world", []byte("Hello World"), 0644)
		if err != nil {
			panic(err)
		}
	} else {
		zap.S().Debugf("hello-world file exists")
	}

	go livenessServer()

	// Wait forever
	select {}
}

var bg = New()

// Returns random liveness status
func livenessHandler(w http.ResponseWriter, req *http.Request) {
	if bg.Bool() {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Internal Server Error")
	}
}

func livenessServer() {
	http.HandleFunc("/health", livenessHandler)
	http.ListenAndServe(":9091", nil)
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
