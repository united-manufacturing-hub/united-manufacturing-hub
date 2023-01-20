package internal

import (
	"github.com/felixge/fgtrace"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strconv"
)

func Initfgtrace() {
	go func() {
		val, set := os.LookupEnv("DEBUG_ENABLE_FGTRACE")
		enabled, err := strconv.ParseBool(val)
		if set && err == nil && enabled {
			zap.S().Warnf("fgtrace is enabled. This might hurt performance !. Set DEBUG_ENABLE_FGTRACE to false to disable.")
			http.DefaultServeMux.Handle("/debug/fgtrace", fgtrace.Config{})
			err := http.ListenAndServe(":1337", nil)
			if err != nil {
				zap.S().Errorf("Failed to start fgtrace: %s", err)
			}
		}
	}()
}
