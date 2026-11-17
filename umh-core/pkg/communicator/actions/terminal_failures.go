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

package actions

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/bridgeadmission"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// Terminal failures of a protocol converter deploy: conditions where waiting
// longer cannot help, so the deploy action fails at once instead of running
// to its timeout. A deploy fails terminally when bridge admission refused the
// bridge for a degraded resource (CPU, memory or disk, each with its own
// error code), or when a flow logged a config error since the deploy started.
// Refusals for the bridge limit or an unproven resource are transient: the
// deploy keeps waiting for those.

// refusalCodes maps the admission causes that make a deploy terminal to the
// error code that names the refused resource. The bridge limit and an
// unproven resource have no code: the deploy waits for those.
var refusalCodes = map[bridgeadmission.Cause]string{
	bridgeadmission.CPU:    models.ErrBridgeRefusedCPU,
	bridgeadmission.Memory: models.ErrBridgeRefusedMemory,
	bridgeadmission.Disk:   models.ErrBridgeRefusedDisk,
}

// deployFailure is a terminal deploy failure: the error code for the frontend
// and the flows to stop.
type deployFailure struct {
	// code is the error code sent to the frontend.
	code    string
	message string
	// flows are the flows to stop. DFCTypeEmpty lets flowsToStop decide.
	flows DFCType
}

// terminalFailure reports a terminal deploy failure, if one has happened: a
// flow logged a config error since startTime, or bridge admission refused the
// bridge for a degraded resource. When both happened, the config error wins:
// it explains what the user must fix, and names the flows to stop.
func terminalFailure(bridgeName string, admission bridgeadmission.Decision, readLogs, writeLogs []s6.LogEntry, startTime time.Time) (deployFailure, bool) {
	if configError, found := findFlowConfigError(readLogs, writeLogs, startTime); found {
		return deployFailure{
			code:    models.ErrConfigFileInvalid,
			message: fmt.Sprintf("Bridge '%s' has an invalid configuration: %s", bridgeName, configError.message),
			flows:   configError.flows,
		}, true
	}

	if code, refused := refusalCodes[admission.Cause]; refused {
		return deployFailure{
			code:    code,
			message: fmt.Sprintf("Bridge '%s' was not started: %s", bridgeName, admission.Message()),
			flows:   DFCTypeEmpty,
		}, true
	}

	return deployFailure{}, false
}

// isTerminalFailureCode reports whether errCode is one of the codes that
// terminalFailure returns.
func isTerminalFailureCode(errCode string) bool {
	if errCode == models.ErrConfigFileInvalid {
		return true
	}

	for _, code := range refusalCodes {
		if code == errCode {
			return true
		}
	}

	return false
}
