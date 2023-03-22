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

package v1

import (
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
)

func TestIsTimerangeEntirelyInTimerangeTrue(t *testing.T) {
	firstTimeRange := TimeRange{
		time.Date(2000, 01, 01, 00, 00, 00, 0, time.UTC),
		time.Date(2010, 01, 01, 00, 00, 00, 0, time.UTC)}
	secondTimeRange := TimeRange{
		time.Date(2000, 01, 01, 00, 00, 00, 0, time.UTC),
		time.Date(2020, 01, 01, 00, 00, 00, 0, time.UTC)}

	if !isTimerangeEntirelyInTimerange(firstTimeRange, secondTimeRange) {
		t.Error()
	}
}

func TestIsTimerangeEntirelyInTimerangeFalse(t *testing.T) {
	firstTimeRange := TimeRange{
		time.Date(2000, 01, 01, 00, 00, 00, 0, time.UTC),
		time.Date(2010, 01, 01, 00, 00, 00, 0, time.UTC)}
	secondTimeRange := TimeRange{
		time.Date(2001, 01, 01, 00, 00, 00, 0, time.UTC),
		time.Date(2020, 01, 01, 00, 00, 00, 0, time.UTC)}

	if isTimerangeEntirelyInTimerange(firstTimeRange, secondTimeRange) {
		t.Error()
	}
}

func getTestShiftPlan() (processedShifts []datamodel.ShiftEntry) {
	beginShift := datamodel.ShiftEntry{
		// first is always no shift
		TimestampBegin: time.Date(1900, 01, 01, 00, 00, 00, 0, time.UTC),
		TimestampEnd:   time.Date(2000, 01, 01, 06, 00, 00, 0, time.UTC),
		ShiftType:      0,
	}
	processedShifts = append(processedShifts, beginShift)

	firstShift := datamodel.ShiftEntry{
		TimestampBegin: time.Date(2000, 01, 01, 06, 00, 00, 0, time.UTC),
		TimestampEnd:   time.Date(2000, 01, 01, 14, 00, 00, 0, time.UTC),
		ShiftType:      1,
	}
	processedShifts = append(processedShifts, firstShift)

	secondShift := datamodel.ShiftEntry{
		// directly after first shift
		TimestampBegin: time.Date(2000, 01, 01, 14, 00, 00, 0, time.UTC),
		TimestampEnd:   time.Date(2000, 01, 01, 22, 00, 00, 0, time.UTC),
		ShiftType:      1,
	}
	processedShifts = append(processedShifts, secondShift)

	noShift := datamodel.ShiftEntry{
		// break between second and third shift
		TimestampBegin: time.Date(2000, 01, 01, 22, 00, 00, 0, time.UTC),
		TimestampEnd:   time.Date(2000, 01, 01, 23, 00, 00, 0, time.UTC),
		ShiftType:      0,
	}
	processedShifts = append(processedShifts, noShift)

	thirdShift := datamodel.ShiftEntry{
		TimestampBegin: time.Date(2000, 01, 01, 23, 00, 00, 0, time.UTC),
		TimestampEnd:   time.Date(2000, 01, 02, 07, 00, 00, 0, time.UTC),
		ShiftType:      1,
	}
	processedShifts = append(processedShifts, thirdShift)

	endShift := datamodel.ShiftEntry{
		// last is always no shift
		TimestampBegin: time.Date(2000, 01, 02, 07, 00, 00, 0, time.UTC),
		TimestampEnd:   time.Date(2200, 01, 01, 00, 00, 00, 0, time.UTC),
		ShiftType:      0,
	}
	processedShifts = append(processedShifts, endShift)

	endShift2 := datamodel.ShiftEntry{
		// last is always no shift
		TimestampBegin: time.Date(2200, 01, 01, 00, 00, 00, 0, time.UTC),
		TimestampEnd:   time.Date(2200, 01, 01, 16, 00, 00, 0, time.UTC),
		ShiftType:      0,
	}
	processedShifts = append(processedShifts, endShift2)

	return
}

/*
func getTestStates() (statesArray []datamodel.StateEntry) {

	statesArray = append(statesArray, datamodel.StateEntry{State: datamodel.UnspecifiedStopState, Timestamp: time.Date(1992, 01, 01, 00, 00, 00, 0, time.UTC)}) // out of shifts
	statesArray = append(statesArray, datamodel.StateEntry{State: datamodel.UnspecifiedStopState, Timestamp: time.Date(2000, 01, 01, 00, 00, 00, 0, time.UTC)})
	statesArray = append(statesArray, datamodel.StateEntry{State: datamodel.ProducingAtFullSpeedState, Timestamp: time.Date(2000, 01, 01, 07, 00, 00, 0, time.UTC)}) // shift starts at 06
	statesArray = append(statesArray, datamodel.StateEntry{State: datamodel.ProducingAtFullSpeedState, Timestamp: time.Date(2000, 01, 01, 07, 30, 00, 0, time.UTC)}) // two times same state
	statesArray = append(statesArray, datamodel.StateEntry{State: datamodel.UnspecifiedStopState, Timestamp: time.Date(2000, 01, 01, 8, 00, 00, 0, time.UTC)})
	statesArray = append(statesArray, datamodel.StateEntry{State: datamodel.ProducingAtFullSpeedState, Timestamp: time.Date(2000, 01, 01, 9, 00, 00, 0, time.UTC)})
	statesArray = append(statesArray, datamodel.StateEntry{State: datamodel.UnspecifiedStopState, Timestamp: time.Date(2000, 01, 01, 15, 00, 00, 0, time.UTC)}) // shift starts at 14

	statesArray = append(statesArray, datamodel.StateEntry{State: datamodel.ProducingAtFullSpeedState, Timestamp: time.Date(2000, 01, 01, 23, 00, 00, 0, time.UTC)})
	statesArray = append(statesArray, datamodel.StateEntry{State: datamodel.UnspecifiedStopState, Timestamp: time.Date(2000, 01, 02, 06, 00, 00, 0, time.UTC)})

	return
}
*/

func TestIsStateEntirelyInNoShift(t *testing.T) {
	shiftPlan := getTestShiftPlan()

	if isStateEntirelyInNoShift(
		datamodel.StateEntry{
			State:     1,
			Timestamp: time.Date(2000, 01, 01, 07, 00, 00, 0, time.UTC)},
		datamodel.StateEntry{State: 1, Timestamp: time.Date(2000, 01, 01, 07, 30, 00, 0, time.UTC)},
		shiftPlan) {
		t.Error("Entirely inside shift")
	}

	if isStateEntirelyInNoShift(
		datamodel.StateEntry{
			State:     1,
			Timestamp: time.Date(2000, 01, 01, 05, 00, 00, 0, time.UTC)},
		datamodel.StateEntry{State: 1, Timestamp: time.Date(2000, 01, 01, 07, 30, 00, 0, time.UTC)},
		shiftPlan) {
		t.Error("On the border between shift and noShift")
	}

	if isStateEntirelyInNoShift(
		datamodel.StateEntry{
			State:     1,
			Timestamp: time.Date(2000, 01, 01, 07, 30, 00, 0, time.UTC)},
		datamodel.StateEntry{State: 1, Timestamp: time.Date(2000, 01, 01, 15, 30, 00, 0, time.UTC)},
		shiftPlan) {
		t.Error("On the border between two shifts")
	}

	if isStateEntirelyInNoShift(
		datamodel.StateEntry{
			State:     1,
			Timestamp: time.Date(2000, 01, 01, 07, 30, 00, 0, time.UTC)},
		datamodel.StateEntry{State: 1, Timestamp: time.Date(2000, 01, 01, 23, 30, 00, 0, time.UTC)},
		shiftPlan) {
		t.Error("On the border between multiple shifts")
	}

	// function assumes that noShifts are aggregated. this is done in cleanRawShiftData
	/*
		if isStateEntirelyInNoShift(datamodel.StateEntry{State: 1, Timestamp: time.Date(2200, 01, 01, 00, 00, 00, 0, time.UTC)}, datamodel.StateEntry{State: 1, Timestamp: time.Date(2200, 01, 01, 15, 00, 00, 0, time.UTC)}, shiftPlan) {
			t.Error("On the border between multiple noShifts")
		}
	*/
	if !isStateEntirelyInNoShift(
		datamodel.StateEntry{
			State:     1,
			Timestamp: time.Date(1990, 01, 01, 07, 00, 00, 0, time.UTC)},
		datamodel.StateEntry{State: 1, Timestamp: time.Date(1991, 01, 01, 07, 30, 00, 0, time.UTC)},
		shiftPlan) {
		t.Error("Entirely outside shift")
	}
}
