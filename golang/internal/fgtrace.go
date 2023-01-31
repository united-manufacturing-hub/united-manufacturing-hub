package internal

import (
	"github.com/felixge/fgtrace"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strconv"
	"time"
)

func Initfgtrace() {
	go func() {
		val, set := os.LookupEnv("DEBUG_ENABLE_FGTRACE")
		if !set {
			zap.S().Infof("DEBUG_ENABLE_FGTRACE not set. Not enabling debug tracing")
			return
		}

		var enabled bool
		var err error
		enabled, err = strconv.ParseBool(val)
		if err != nil {
			zap.S().Errorf("DEBUG_ENABLE_FGTRACE is not a valid boolean: %s", val)
			return
		}
		if enabled {
			zap.S().Warnf("fgtrace is enabled. This might hurt performance !. Set DEBUG_ENABLE_FGTRACE to false to disable.")
			http.DefaultServeMux.Handle("/debug/fgtrace", fgtrace.Config{})
			server := &http.Server{
				Addr:              ":1337",
				ReadHeaderTimeout: 3 * time.Second,
			}
			errX := server.ListenAndServe()
			if errX != nil {
				zap.S().Errorf("Failed to start fgtrace: %s", errX)
			}
		} else {
			zap.S().Debugf("Debug Tracing is disabled. Set DEBUG_ENABLE_FGTRACE to true to enable.")
		}
	}()
}
