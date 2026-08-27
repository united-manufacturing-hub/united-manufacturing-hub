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

// What the CPU worker's observation passed through during a run, and how to
// find one reading in it. The three CPU scenario specs beside this file all
// need it: each stages a sequence of machine conditions, and the readings those
// produce do not coexist.

package examples_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
)

// cpuObservation is one state the CPU worker's observation passed through: what
// the worker judged, and the framework's own verdict on the poll that produced
// it. Reason and Degraded come from the framework rather than from cpuhealth,
// and they are the only place a FAILED poll shows up — a poll that errors
// publishes an empty Verdict and Message, so a spec reading only those two
// cannot tell a failed read from a worker that has not started.
type cpuObservation struct {
	Message  string
	Verdict  string
	Reason   string
	Degraded bool
}

// cpuObservationHistory replays the store's delta history into the states the
// CPU worker's observation passed through, oldest first.
//
// A spec reaches for this when the readings it has to check do not coexist. The
// worker's observation holds one state at a time and the runner hands a spec
// back a finished run, so anything a driver staged before the last condition is
// unreachable through LoadObservedTyped. The delta history is the store's own
// record of every field change, and reading it here is the ONLY way to see the
// sequence: --dump-store cannot show it, because runV2 ignores the flag and
// warns dump_store_not_supported_for_v2, and every CPU scenario is v2. So a
// spec sees readings a reader of the command line does not.
//
// Replaying rather than reading each delta alone is what makes a state whole: a
// delta carries only the fields that changed, so a reading whose verdict moved
// while its message did not would otherwise arrive with an empty message.
//
// Only CHANGES are kept. The worker publishes an observation on every poll and
// its timestamp moves each time, so the store holds a delta per poll and one
// verdict spans hundreds of them. Keeping the repeats would make "this sentence
// came after that one" true of any two sentences that overlapped by a single
// reading, which is most of them, and an ordering check that cannot fail is not
// one. A state that comes BACK after a different state is a fresh entry, which
// is what a re-fired verdict needs.
//
// GetDeltas serves a bounded page, so this pages to the end rather than
// reporting the first hundred changes as the whole run.
func cpuObservationHistory(store storage.TriangularStoreInterface) []cpuObservation {
	GinkgoHelper()

	var (
		history    []cpuObservation
		current    cpuObservation
		lastSyncID int64
	)

	for {
		resp, err := store.GetDeltas(context.Background(), storage.Subscription{LastSyncID: lastSyncID})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.RequiresBootstrap).To(BeFalse(),
			"the store dropped the delta history this check reads")

		if len(resp.Deltas) == 0 {
			break
		}

		for _, delta := range resp.Deltas {
			lastSyncID = delta.SyncID

			if delta.WorkerType != fsmv2cpu.WorkerType || delta.Role != storage.RoleObserved || delta.Changes == nil {
				continue
			}

			previous := current
			applyCPUChange(&current, delta.Changes)

			if len(history) == 0 || current != previous {
				history = append(history, current)
			}
		}

		if !resp.HasMore {
			break
		}
	}

	return history
}

// applyCPUChange folds one delta's changed fields into the running state. The
// first observation ADDS every field and later ones MODIFY the few that moved,
// so both maps are read.
func applyCPUChange(into *cpuObservation, changes *storage.Diff) {
	set := func(field string, value interface{}) {
		switch field {
		case "message":
			into.Message = fmt.Sprint(value)
		case "verdict":
			into.Verdict = fmt.Sprint(value)
		case "reason":
			into.Reason = fmt.Sprint(value)
		case "degraded":
			into.Degraded = value == true
		}
	}

	for field, value := range changes.Added {
		set(field, value)
	}

	for field, modified := range changes.Modified {
		set(field, modified.New)
	}
}

// messageAfter returns the position of the first reading past after whose
// message holds substring, or -1 when none does. Pass -1 for after to search
// the whole run.
//
// The position is what a caller compares, so two sentences that arrived in the
// wrong order fail rather than both passing on being present. Searching from a
// position rather than from the start is also what lets a caller check a
// sentence that comes BACK: the second time a machine says the same thing is a
// different claim from the first, and looking from the start would find the
// first.
func messageAfter(history []cpuObservation, after int, substring string) int {
	return indexAfter(history, after, substring, func(o cpuObservation) string { return o.Message })
}

// reasonAfter is messageAfter over the framework's own reason, which is where a
// failed poll is reported.
func reasonAfter(history []cpuObservation, after int, substring string) int {
	return indexAfter(history, after, substring, func(o cpuObservation) string { return o.Reason })
}

func indexAfter(history []cpuObservation, after int, substring string, field func(cpuObservation) string) int {
	for i := after + 1; i < len(history); i++ {
		if strings.Contains(field(history[i]), substring) {
			return i
		}
	}

	return -1
}
