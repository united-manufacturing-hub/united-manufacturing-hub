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
