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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
)

// actionPanicsTotal counts panics recovered in the Communicator action
// handler. Labels record which action type panicked and how the recovered
// value was classified (matches panicutil.PanicType* in the FSMv2 supervisor
// so dashboards can treat action panics and worker panics uniformly).
var actionPanicsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "umh_communicator_action_panics_total",
		Help: "Total number of panics recovered in action handlers, by action type and panic class.",
	},
	[]string{"action_type", "panic_type"},
)

// RecordActionPanic increments the recovered-action-panic counter.
// actionType is the ActionMessagePayload.ActionType that was executing when
// the panic occurred. panicType is one of "error_panic", "string_panic", or
// "unknown_panic".
func RecordActionPanic(actionType, panicType string) {
	actionPanicsTotal.WithLabelValues(actionType, panicType).Inc()
}

// ActionPanicsTotalForTest returns the current value of the action-panic
// counter for a given (actionType, panicType) label pair. Exported so
// callers outside this package — specifically the Communicator action
// handler tests — can assert that recovery paths increment the metric.
// Not intended for production code.
func ActionPanicsTotalForTest(actionType, panicType string) float64 {
	counter, err := actionPanicsTotal.GetMetricWithLabelValues(actionType, panicType)
	if err != nil {
		return 0
	}

	var pb dto.Metric

	if err := counter.Write(&pb); err != nil {
		return 0
	}

	return pb.GetCounter().GetValue()
}
