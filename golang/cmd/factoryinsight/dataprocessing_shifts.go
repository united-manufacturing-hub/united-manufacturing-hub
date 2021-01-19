package main

import (
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
)

// TimeRange contains a time range
type TimeRange struct {
	Begin time.Time
	End   time.Time
}

func isTimerangeEntirelyInTimerange(firstTimeRange TimeRange, secondTimeRange TimeRange) bool {
	if (firstTimeRange.Begin.After(secondTimeRange.Begin) || firstTimeRange.Begin == secondTimeRange.Begin) && (firstTimeRange.End.Before(secondTimeRange.End) || firstTimeRange.End == secondTimeRange.End) {
		return true
	}
	return false
}

func isTimepointInTimerange(timestamp time.Time, secondTimeRange TimeRange) bool {
	if (timestamp.After(secondTimeRange.Begin)) && (timestamp.Before(secondTimeRange.End)) {
		return true
	}
	return false
}

func isStateEntirelyInNoShift(state datamodel.StateEntry, followingState datamodel.StateEntry, processedShifts []datamodel.ShiftEntry) bool {
	firstTimeRange := TimeRange{state.Timestamp, followingState.Timestamp}

	// Loop through all shifts
	for _, dataPoint := range processedShifts {
		if dataPoint.ShiftType == 0 {
			secondTimeRange := TimeRange{dataPoint.TimestampBegin, dataPoint.TimestampEnd}
			if isTimerangeEntirelyInTimerange(firstTimeRange, secondTimeRange) {
				return true
			}
		}
	}

	return false
}

func getOverlappingShifts(state datamodel.StateEntry, followingState datamodel.StateEntry, processedShifts []datamodel.ShiftEntry) (overlappingShifts []datamodel.ShiftEntry) {
	firstTimeRange := TimeRange{state.Timestamp, followingState.Timestamp}

	// Loop through all shifts
	for _, dataPoint := range processedShifts {
		if dataPoint.ShiftType != 0 {
			secondTimeRange := TimeRange{dataPoint.TimestampBegin, dataPoint.TimestampEnd}

			// if firstTimeRange.Begin in TimeRange or firstTimeRange.End in TimeRange
			if isTimepointInTimerange(firstTimeRange.Begin, secondTimeRange) || isTimepointInTimerange(firstTimeRange.End, secondTimeRange) { // if state begin is in time range or state end is in time range
				overlappingShifts = append(overlappingShifts, dataPoint)
			}
		}
	}

	return
}

func isStateEntirelyOutsideNoShift(state datamodel.StateEntry, followingState datamodel.StateEntry, processedShifts []datamodel.ShiftEntry) bool {
	firstTimeRange := TimeRange{state.Timestamp, followingState.Timestamp}

	// Loop through all shifts
	for _, dataPoint := range processedShifts {
		if dataPoint.ShiftType != 0 { //if shift is anything else other than no shift
			secondTimeRange := TimeRange{dataPoint.TimestampBegin, dataPoint.TimestampEnd}
			if isTimerangeEntirelyInTimerange(firstTimeRange, secondTimeRange) {
				return true
			}
		}
	}

	return false
}

// Adds shifts with id
func addNoShiftsBetweenShifts(shiftArray []datamodel.ShiftEntry, configuration datamodel.CustomerConfiguration) (processedShifts []datamodel.ShiftEntry) {

	// Loop through all datapoints
	for index, dataPoint := range shiftArray {
		if index > 0 { //if not the first entry, add a noShift

			previousDataPoint := shiftArray[index-1]
			timestampBegin := previousDataPoint.TimestampEnd
			timestampEnd := dataPoint.TimestampBegin

			if timestampBegin != timestampEnd { // timestampBegin == timestampEnd ahppens when a no shift is already in the list.
				// TODO: Fix
				fullRow := datamodel.ShiftEntry{
					TimestampBegin: timestampBegin,
					TimestampEnd:   timestampEnd,
					ShiftType:      0, //shiftType =0 is noShift
				}
				processedShifts = append(processedShifts, fullRow)
			}
		}
		fullRow := datamodel.ShiftEntry{
			TimestampBegin: dataPoint.TimestampBegin,
			TimestampEnd:   dataPoint.TimestampEnd,
			ShiftType:      dataPoint.ShiftType,
		}
		processedShifts = append(processedShifts, fullRow)

	}
	return
}

// cleanRawShiftData has a complicated algorithm, so here a human readable explaination
// Function 1: it prevents going shifts out of selected time range
// Function 2: if there are multiple shifts adjacent to each other with a given tolerance of 10 minutes, it combines them
func cleanRawShiftData(shiftArray []datamodel.ShiftEntry, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (processedShifts []datamodel.ShiftEntry) {

	var timestampBegin time.Time
	var timestampEnd time.Time
	var shiftType int

	for index, dataPoint := range shiftArray {
		// first and last shift are always of type noShift. Therefore, ignore them here
		if dataPoint.ShiftType == 0 {
			fullRow := datamodel.ShiftEntry{
				TimestampBegin: dataPoint.TimestampBegin,
				TimestampEnd:   dataPoint.TimestampEnd,
				ShiftType:      0, //shiftType =0 is noShift
			}
			processedShifts = append(processedShifts, fullRow)
			continue
		}

		// set timestampBegin and timestampEnd if not set yet
		if timestampBegin == (time.Time{}) || timestampEnd == (time.Time{}) {
			timestampBegin = dataPoint.TimestampBegin
			timestampEnd = dataPoint.TimestampEnd
		}

		// if not last entry
		if index < len(shiftArray)-1 {
			nextShift := shiftArray[index+1]

			// if following shift is adjacent to currentShiftEndTimestamp
			toleranceTimeRange := TimeRange{
				Begin: nextShift.TimestampBegin.Add(-time.Duration(5) * time.Minute),
				End:   nextShift.TimestampBegin.Add(time.Duration(5) * time.Minute), // should not happend as it is forbidden by the datamodel (prevent_overlap check)
			}

			if isTimepointInTimerange(timestampEnd, toleranceTimeRange) && nextShift.ShiftType != 0 { // if its is in rimerange and the following shift is not noShift

				// adjust timestampEnd
				timestampEnd = nextShift.TimestampEnd
			} else { // otherwise
				// add it
				shiftType = dataPoint.ShiftType

				fullRow := datamodel.ShiftEntry{
					TimestampBegin: timestampBegin,
					TimestampEnd:   timestampEnd,
					ShiftType:      shiftType, //shiftType =0 is noShift
				}
				processedShifts = append(processedShifts, fullRow)

				// reset timestampBegin and timestampEnd
				timestampBegin = (time.Time{})
				timestampEnd = (time.Time{})
			}

		} else { // if last entry

			// add ongoing shift
			if timestampBegin != (time.Time{}) || timestampEnd != (time.Time{}) {
				// add no shift at the end
				fullRow := datamodel.ShiftEntry{
					TimestampBegin: timestampBegin,
					TimestampEnd:   timestampEnd,
					ShiftType:      dataPoint.ShiftType, //shiftType =0 is noShift
				}
				processedShifts = append(processedShifts, fullRow)
			}
			continue
		}
	}
	return
}

func recursiveSplittingOfShiftsToAddNoShifts(dataPoint datamodel.StateEntry, followingDataPoint datamodel.StateEntry, processedShifts []datamodel.ShiftEntry, processedStateArrayRaw []datamodel.StateEntry, executionAmount int) (processedStateArray []datamodel.StateEntry) {
	var state int
	var timestamp time.Time

	overlappingShifts := getOverlappingShifts(dataPoint, followingDataPoint, processedShifts)
	// Loop through all shifts
	/*
		for _, shift := range overlappingShifts {
			zap.S().Infof("Overlapping shift ", shift.TimestampBegin, shift.TimestampEnd, dataPoint.Timestamp, len(overlappingShifts))
		}
	*/

	executionAmount++
	if executionAmount > 10 { // prevent executing the function too often
		processedStateArray = processedStateArrayRaw
		zap.S().Errorf("Executed loop too often ", executionAmount)
		return
	}

	if len(overlappingShifts) > 0 { // if there are overlapping shifts
		//zap.S().Infof("len(overlappingShifts) ", len(overlappingShifts))
		if dataPoint.Timestamp.Before(overlappingShifts[0].TimestampBegin) { // if the beginning of the state is out of the shift
			//zap.S().Infof("## State before shift begin ", overlappingShifts[0].TimestampBegin, overlappingShifts[0].TimestampEnd, dataPoint.Timestamp, followingDataPoint.Timestamp)
			// add everything till shift begin as "noShift"
			timestamp = dataPoint.Timestamp
			state = datamodel.NoShiftState
			fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
			//zap.S().Infof("Added state ", timestamp, state)
			processedStateArray = append(processedStateArrayRaw, fullRow)

			// Execute same function for the rest
			timestamp = overlappingShifts[0].TimestampBegin
			state = dataPoint.State
			fullRow = datamodel.StateEntry{State: state, Timestamp: timestamp}

			if len(overlappingShifts) == 1 { // if last seperation, abort
				//zap.S().Infof("## EXIT 1 ", timestamp, state)
				processedStateArray = append(processedStateArray, fullRow)
			} else { // otherwise continue
				processedStateArray = recursiveSplittingOfShiftsToAddNoShifts(fullRow, followingDataPoint, processedShifts, processedStateArray, executionAmount)
			}

		} else { // if the end of the state is out of the shift. Therefore, the beginning of the state is still in the shift.
			//zap.S().Infof("## State after shift begin ", overlappingShifts[0].TimestampBegin, overlappingShifts[0].TimestampEnd, dataPoint.Timestamp, followingDataPoint.Timestamp)

			timestamp = dataPoint.Timestamp
			state = dataPoint.State
			fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
			//zap.S().Infof("Added state ", timestamp, state)
			processedStateArray = append(processedStateArrayRaw, fullRow) // add the datapoint with its corresponding state

			// we need to deep dive further in here
			timestamp = overlappingShifts[0].TimestampEnd

			if len(overlappingShifts) == 1 { // if last seperation, abort
				state = datamodel.NoShiftState
				fullRow = datamodel.StateEntry{State: state, Timestamp: timestamp}
				//zap.S().Infof("## EXIT 2 ", timestamp, state)
				processedStateArray = append(processedStateArray, fullRow)
			} else { // otherwise continue
				state = dataPoint.State
				fullRow = datamodel.StateEntry{State: state, Timestamp: timestamp}
				processedStateArray = recursiveSplittingOfShiftsToAddNoShifts(fullRow, followingDataPoint, processedShifts, processedStateArray, executionAmount)
			}

		}
	} else {

		timestamp = dataPoint.Timestamp
		state = dataPoint.State
		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		//zap.S().Infof("## EXIT 3 ", timestamp, state)
		processedStateArray = append(processedStateArrayRaw, fullRow)
	}
	return
}

func addNoShiftsToStates(parentSpan opentracing.Span, rawShifts []datamodel.ShiftEntry, stateArray []datamodel.StateEntry, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	// Jaeger tracing
	if parentSpan != nil { //nil during testing
		span := opentracing.StartSpan(
			"addNoShiftsToStates",
			opentracing.ChildOf(parentSpan.Context()))
		defer span.Finish()
	}

	processedShifts := cleanRawShiftData(rawShifts, from, to, configuration)
	processedShifts = addNoShiftsBetweenShifts(processedShifts, configuration)

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if datamodel.IsProducing(dataPoint.State) { //if running, do not do anything
			fullRow := datamodel.StateEntry{
				State:     dataPoint.State,
				Timestamp: dataPoint.Timestamp,
			}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		var followingDataPoint datamodel.StateEntry

		if index == len(stateArray)-1 { //if last entry, ignore
			followingDataPoint = datamodel.StateEntry{
				State:     -1,
				Timestamp: to,
			}
		} else {
			followingDataPoint = stateArray[index+1]
		}

		// TODO: parallelize and work with go and channels

		if isStateEntirelyInNoShift(dataPoint, followingDataPoint, processedShifts) {
			state = datamodel.NoShiftState //noShift
		} else if isStateEntirelyOutsideNoShift(dataPoint, followingDataPoint, processedShifts) {
			state = dataPoint.State
		} else { // now we have a state that is somehow overlapping with shifts and which we need to split up
			processedStateArray = recursiveSplittingOfShiftsToAddNoShifts(dataPoint, followingDataPoint, processedShifts, processedStateArray, 0)
			continue
		}

		timestamp = dataPoint.Timestamp

		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}
