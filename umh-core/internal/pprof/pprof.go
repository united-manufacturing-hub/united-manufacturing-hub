// Copyright 2025 UMH Systems GmbH
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

//go:build pprof
// +build pprof

package pprof

// Registers the pprof handlers on the default ServeMux when the tag is set.
import (
	"net/http"
	_ "net/http/pprof"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

// Open webserver for pprof
func StartPprofServer() {
	logger.For(logger.ComponentCore).Info("Starting pprof server on port 6060")
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}
