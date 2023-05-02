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
	"github.com/united-manufacturing-hub/umh-utils/env"
	"go.uber.org/zap"
	"net/http"
	"time"
)

func Initfgtrace() {
	go func() {
		enabled, err := env.GetAsBool("DEBUG_ENABLE_FGTRACE", false, false)
		if err != nil {
			zap.S().Fatal(err)
		}
		if !enabled {
			zap.S().Infof("DEBUG_ENABLE_FGTRACE not set. Not enabling debug tracing")
			return
		}
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
	}()
}
