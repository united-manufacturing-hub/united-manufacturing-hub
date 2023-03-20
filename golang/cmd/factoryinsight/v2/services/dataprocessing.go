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

package services

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"gonum.org/v1/gonum/stat"
	"math"
	"sort"
	"time"
)

// ConvertStateToString converts a state in integer format to a human-readable string
func ConvertStateToString(state int, configuration datamodel.EnterpriseConfiguration) (stateString string) {

	languageCode := configuration.LanguageCode

	stateString = datamodel.ConvertStateToString(state, languageCode)

	return
}

// ConvertActivityToString converts a maintenance activity in integer format to a human-readable string
func ConvertActivityToString(activity int, configuration datamodel.EnterpriseConfiguration) (activityString string) {

	languageCode := configuration.LanguageCode

	if languageCode == 0 {
		switch activity {
		case 0:
			activityString = "Inspection"
		case 1:
			activityString = "Austausch"
		default:
			activityString = fmt.Sprintf("Unbekannte Aktivität mit Code %d", activity)
		}
	} else {
		switch activity {
		case 0:
			activityString = "Inspection"
		case 1:
			activityString = "Replacement"
		default:
			activityString = fmt.Sprintf("Unknown activity with code %d", activity)
		}
	}

	return
}

// CalculateDurations returns an array with the duration between the states.
func CalculateDurations(
	temporaryDatapoints []datamodel.StateEntry,
	to time.Time,
	returnChannel chan datamodel.ChannelResult) {

	// Prepare datamodel.ChannelResult
	durations := make([]float64, 0, len(temporaryDatapoints))
	var err error

	// Loop through all datapoints
	for index, datapoint := range temporaryDatapoints {
		var timestampAfterCurrentOne time.Time
		// Special handling of last datapoint
		if index >= len(temporaryDatapoints)-1 {
			timestampAfterCurrentOne = to
		} else { // Get the following datapoint
			datapointAfterCurrentOne := temporaryDatapoints[index+1]
			timestampAfterCurrentOne = datapointAfterCurrentOne.Timestamp
		}

		timestampCurrent := datapoint.Timestamp
		if timestampAfterCurrentOne.Sub(timestampCurrent).Seconds() < 0 {

			err = errors.New("timestampAfterCurrentOne.Sub(timestampCurrent).Seconds() < 0 detected")
			BusinessLogicErrorHandling("calculateDurations", err, false)
			zap.S().Errorw(
				"timestampAfterCurrentOne.Sub(timestampCurrent).Seconds() < 0",
				"timestampAfterCurrentOne.Sub(timestampCurrent).Seconds()",
				timestampAfterCurrentOne.Sub(timestampCurrent).Seconds(),
				"timestampAfterCurrentOne",
				timestampAfterCurrentOne,
				"timestampCurrent",
				timestampCurrent,
				"state",
				datapoint.State,
			)
		}
		durations = append(durations, timestampAfterCurrentOne.Sub(timestampCurrent).Seconds())
	}

	// Send datamodel.ChannelResult back
	var ChannelResultInstance datamodel.ChannelResult
	ChannelResultInstance.Err = err
	ChannelResultInstance.ReturnValue = durations
	returnChannel <- ChannelResultInstance
}

func TransformToStateArray(temporaryDatapoints []datamodel.StateEntry, returnChannel chan datamodel.ChannelResult) {

	// Prepare datamodel.ChannelResult
	stateArray := make([]int, 0, len(temporaryDatapoints))
	var err error

	// Loop through all datapoints
	for _, datapoint := range temporaryDatapoints {
		stateArray = append(stateArray, datapoint.State)
	}

	// Send datamodel.ChannelResult back
	var ChannelResultInstance datamodel.ChannelResult
	ChannelResultInstance.Err = err
	ChannelResultInstance.ReturnValue = stateArray
	returnChannel <- ChannelResultInstance
}

func GetTotalDurationForState(
	durationArray []float64,
	stateArray []int,
	state int,
	returnChannel chan datamodel.ChannelResult) {

	// Prepare datamodel.ChannelResult
	var totalDuration float64
	var err error

	totalDuration = 0

	// Loop through all datapoints and sum up total duration
	for index, datapoint := range stateArray {
		if datapoint == state {
			totalDuration += durationArray[index]
			if durationArray[index] < 0 {
				err = fmt.Errorf("durationArray[index] < 0: %f", durationArray[index])
				BusinessLogicErrorHandling("getTotalDurationForState", err, false)
			}
		}
	}

	var ParetoEntry datamodel.ParetoEntry
	ParetoEntry.Duration = totalDuration
	ParetoEntry.State = state

	// Send datamodel.ChannelResult back
	var ChannelResultInstance datamodel.ChannelResult
	ChannelResultInstance.Err = err
	ChannelResultInstance.ReturnValue = ParetoEntry
	returnChannel <- ChannelResultInstance
}

func AddUnknownMicrostops(
	stateArray []datamodel.StateEntry,
	configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry) {

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if datamodel.IsProducing(dataPoint.State) { // if running, do not do anything
			fullRow := datamodel.StateEntry{
				State:     dataPoint.State,
				Timestamp: dataPoint.Timestamp,
			}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 || index == 0 { // if last entry or first entry, ignore
			fullRow := datamodel.StateEntry{
				State:     dataPoint.State,
				Timestamp: dataPoint.Timestamp,
			}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]

		stateDuration := followingDataPoint.Timestamp.Sub(dataPoint.Timestamp).Seconds()

		timestamp = dataPoint.Timestamp

		if stateDuration <= configuration.MicrostopDurationInSeconds && datamodel.IsUnspecifiedStop(dataPoint.State) { // if duration smaller than configured threshold AND unknown stop
			state = datamodel.MicrostopState // microstop
		} else {
			state = dataPoint.State
		}

		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

func GetProducedPiecesFromCountSlice(countSlice []datamodel.CountEntry, from, to time.Time) (totalCount float64) {

	// Loop through all datapoints
	for _, dataPoint := range countSlice {
		count := dataPoint.Count
		timestamp := dataPoint.Timestamp

		if IsTimepointInTimerange(timestamp, TimeRange{from, to}) {
			totalCount += count
		}
	}
	return
}

func RemoveUnnecessaryElementsFromCountSlice(
	countSlice []datamodel.CountEntry,
	from, to time.Time) (processedCountSlice []datamodel.CountEntry) {
	if len(countSlice) == 0 {
		return
	}
	// Loop through all datapoints
	for _, dataPoint := range countSlice {
		if IsTimepointInTimerange(dataPoint.Timestamp, TimeRange{from, to}) {
			processedCountSlice = append(processedCountSlice, dataPoint)
		}
	}
	return
}

func RemoveUnnecessaryElementsFromOrderArray(
	orderArray []datamodel.OrdersRaw,
	from, to time.Time) (processedOrdersArray []datamodel.OrdersRaw) {
	if len(orderArray) == 0 {
		return
	}
	// Loop through all datapoints
	for _, dataPoint := range orderArray {
		if IsTimepointInTimerange(
			dataPoint.BeginTimestamp,
			TimeRange{from, to}) || IsTimepointInTimerange(dataPoint.EndTimestamp, TimeRange{from, to}) {
			processedOrdersArray = append(processedOrdersArray, dataPoint)
		}
	}
	return
}

func RemoveUnnecessaryElementsFromStateSlice(
	processedStatesRaw []datamodel.StateEntry,
	from, to time.Time) (processedStates []datamodel.StateEntry) {
	if len(processedStatesRaw) == 0 {
		return
	}
	firstSelectedTimestampIndex := -1
	// Loop through all datapoints
	for index, dataPoint := range processedStatesRaw {
		// if is state in range or equal to from or to time range
		if IsTimepointInTimerange(
			dataPoint.Timestamp,
			TimeRange{from, to}) || dataPoint.Timestamp == from || dataPoint.Timestamp == to {

			if firstSelectedTimestampIndex == -1 { // remember the first selected element
				firstSelectedTimestampIndex = index
			}

			processedStates = append(processedStates, dataPoint)
		}
	}

	if len(processedStates) > 0 && processedStates[0].Timestamp.After(from) { // if there is time missing between from and the first selected timestamp, add the element just before the first selected timestamp

		if firstSelectedTimestampIndex == 0 { // there is data missing here, throwing a warning
			zap.S().Warnf("data missing, firstSelectedTimestampIndex == 0", processedStates[0].Timestamp)
		} else {

			newDataPoint := datamodel.StateEntry{}
			newDataPoint.Timestamp = from
			newDataPoint.State = processedStatesRaw[firstSelectedTimestampIndex-1].State

			processedStates = append(
				[]datamodel.StateEntry{newDataPoint},
				processedStates...) // prepand = put it as first element. reference: https://medium.com/@tzuni_eh/go-append-prepend-item-into-slice-a4bf167eb7af
		}

	}

	// this subflow is needed to calculate noShifts while using processStatesOptimized. See also #106

	if len(processedStates) == 0 { // if no value in time range take the previous time stamp.
		previousDataPoint := datamodel.StateEntry{}

		// if there is only one element in it, use it (before taking the performance intensive way further down)
		if len(processedStatesRaw) == 1 {
			newDataPoint := datamodel.StateEntry{}
			newDataPoint.Timestamp = from
			newDataPoint.State = processedStatesRaw[0].State

			processedStates = append(processedStates, newDataPoint)
			return
		}

		// Loop through all datapoints
		for index, dataPoint := range processedStatesRaw {
			// if the current timestamp is after the start of the time range
			if dataPoint.Timestamp.After(from) {
				// we have found the previous timestamp and add it
				newDataPoint := datamodel.StateEntry{}

				newDataPoint.Timestamp = from

				if index > 0 {
					newDataPoint.State = previousDataPoint.State
				} else {
					newDataPoint.State = dataPoint.State
				}

				processedStates = append(processedStates, newDataPoint)

				// no need to continue now, aborting
				return
			}

			previousDataPoint = dataPoint
		}

		// if nothing has been found so far, use the last element (reason: there is no state after "from")
		lastElement := processedStatesRaw[len(processedStatesRaw)-1] // last element in the row
		newDataPoint := datamodel.StateEntry{}
		newDataPoint.Timestamp = from
		newDataPoint.State = lastElement.State
		processedStates = append(processedStates, newDataPoint)

	}
	return
}

// CalculatateLowSpeedStates splits up a "Running" state into multiple states either "Running" or "LowSpeed"
// additionally it caches it results. See also cache.go
func CalculatateLowSpeedStates(
	workCellId uint32,
	countSlice []datamodel.CountEntry,
	from, to time.Time,
	configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry) {

	// Get from cache if possible
	processedStateArray, cacheHit := internal.GetCalculatateLowSpeedStatesFromCache(from, to, workCellId)
	if cacheHit {
		return
	}

	countSlice = RemoveUnnecessaryElementsFromCountSlice(
		countSlice,
		from,
		to) // remove unnecessary items (items outside current state) to improve speed

	var lastState int

	lastState = -1

	oldD := from

	for d := from; !d.After(to); d = d.Add(time.Minute) { // timestamp is beginning of the state. d is current progress.
		if d == oldD { // if first entry
			continue
		}

		averageProductionSpeedPerMinute := GetProducedPiecesFromCountSlice(countSlice, oldD, d)

		if averageProductionSpeedPerMinute >= configuration.LowSpeedThresholdInPcsPerHour/60 { // if this minute is running at full speed
			if !datamodel.IsProducingFullSpeed(lastState) { // if the state is not already running, create new state
				fullRow := datamodel.StateEntry{
					State:     datamodel.ProducingAtFullSpeedState,
					Timestamp: oldD,
				}
				lastState = datamodel.ProducingAtFullSpeedState
				processedStateArray = append(processedStateArray, fullRow)
			}
		} else { // if this minute is "LowSpeed"
			if !datamodel.IsProducingLowerThanFullSpeed(lastState) { // if the state is not already LowSpeed, create new state
				fullRow := datamodel.StateEntry{
					State:     datamodel.ProducingAtLowerThanFullSpeedState,
					Timestamp: oldD,
				}
				lastState = datamodel.ProducingAtLowerThanFullSpeedState
				processedStateArray = append(processedStateArray, fullRow)
			}
		}

		oldD = d
	}

	// Store in cache for later usage
	internal.StoreCalculatateLowSpeedStatesToCache(from, to, workCellId, processedStateArray)

	return
}

// Note: assetID is only used for caching
func AddLowSpeedStates(
	workCellId uint32,
	stateArray []datamodel.StateEntry,
	countSlice []datamodel.CountEntry,
	configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry) {

	// actual function start
	// TODO: neglecting all other states with additional information, e.g. 10556

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if !datamodel.IsProducing(dataPoint.State) { // if not running, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 { // if last entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]
		stateDuration := followingDataPoint.Timestamp.Sub(dataPoint.Timestamp).Minutes()

		timestamp = dataPoint.Timestamp

		averageProductionSpeedPerMinute := GetProducedPiecesFromCountSlice(
			countSlice,
			timestamp,
			followingDataPoint.Timestamp) / stateDuration

		if averageProductionSpeedPerMinute < configuration.LowSpeedThresholdInPcsPerHour/60 {
			rows := CalculatateLowSpeedStates(
				workCellId,
				countSlice,
				timestamp,
				followingDataPoint.Timestamp,
				configuration)
			// Add all states
			processedStateArray = append(processedStateArray, rows...)

		} else {
			state = dataPoint.State
			fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
			processedStateArray = append(processedStateArray, fullRow)
		}

	}

	return
}

func SpecifySmallNoShiftsAsBreaks(
	stateArray []datamodel.StateEntry,
	configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry) {

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if !datamodel.IsNoShift(dataPoint.State) { // if not noShift, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 { // if last entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]

		stateDuration := followingDataPoint.Timestamp.Sub(dataPoint.Timestamp).Seconds()

		timestamp = dataPoint.Timestamp

		if stateDuration <= configuration.ThresholdForNoShiftsConsideredBreakInSeconds { // if duration smaller than configured threshold AND unknown stop
			state = datamodel.OperatorBreakState // Break
		} else {
			state = dataPoint.State
		}

		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

func RemoveSmallRunningStates(
	stateArray []datamodel.StateEntry,
	configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry) {

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if !datamodel.IsProducing(dataPoint.State) { // if not running, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 || index == 0 { // if last entry or first entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]

		stateDuration := followingDataPoint.Timestamp.Sub(dataPoint.Timestamp).Seconds()

		timestamp = dataPoint.Timestamp
		state = datamodel.ProducingAtFullSpeedState

		if stateDuration <= configuration.MinimumRunningTimeInSeconds { // if duration smaller than configured threshold
			continue // do not add it
		}

		// otherwise, add the running time
		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

func RemoveSmallStopStates(
	stateArray []datamodel.StateEntry,
	configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry) {

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if datamodel.IsProducing(dataPoint.State) { // if running, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 || index == 0 { // if last entry or first entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]

		stateDuration := followingDataPoint.Timestamp.Sub(dataPoint.Timestamp).Seconds()

		timestamp = dataPoint.Timestamp
		state = dataPoint.State

		if stateDuration <= configuration.IgnoreMicrostopUnderThisDurationInSeconds { // if duration smaller than configured threshold
			continue // do not add it
		}

		// otherwise, add the running time
		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

func CombineAdjacentStops(stateArray []datamodel.StateEntry) (processedStateArray []datamodel.StateEntry) {

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if datamodel.IsProducing(dataPoint.State) { // if running, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == 0 { // if first entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		previousDataPoint := stateArray[index-1]

		// if the current stop is an unknown stop and the previous one is not running (unspecified or specified stop) and not noShift (or break)
		if datamodel.IsUnspecifiedStop(dataPoint.State) && !datamodel.IsProducing(previousDataPoint.State) && !datamodel.IsNoShift(previousDataPoint.State) && !datamodel.IsOperatorBreak(previousDataPoint.State) {
			continue // then don't add the current state (it gives no additional information). As a result we remove adjacent unknown stops
		}

		// if the state is the same state as the previous one, then don't add it. Theoretically not possible. Practically happened several times.
		if dataPoint.State == previousDataPoint.State {
			continue
		}

		timestamp = dataPoint.Timestamp
		state = dataPoint.State

		// otherwise, add the state
		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

func SpecifyUnknownStopsWithFollowingStopReason(stateArray []datamodel.StateEntry) (processedStateArray []datamodel.StateEntry) {

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if datamodel.IsProducing(dataPoint.State) { // if running or no shift, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 { // if last entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]
		timestamp = dataPoint.Timestamp

		if datamodel.IsUnspecifiedStop(dataPoint.State) && !datamodel.IsNoShift(followingDataPoint.State) && !datamodel.IsOperatorBreak(followingDataPoint.State) && datamodel.IsSpecifiedStop(followingDataPoint.State) { // if the following state is a specified stop that is not noShift AND the current is unknown stop
			state = followingDataPoint.State // then the current state uses the same specification
		} else {
			state = dataPoint.State // otherwise, use the state
		}

		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

// Adds noOrders at the beginning, ending and between orders
func AddNoOrdersBetweenOrders(
	orderArray []datamodel.OrdersRaw,
	from, to time.Time) (processedOrders []datamodel.OrderEntry) {

	// Loop through all datapoints
	for index, dataPoint := range orderArray {

		// if first entry and no order has started yet, then add no order till first order starts
		if index == 0 && dataPoint.BeginTimestamp.After(from) {

			newTimestampEnd := dataPoint.BeginTimestamp.Add(time.Duration(-1) * time.Millisecond) // end it one Millisecond before the next orders starts

			fullRow := datamodel.OrderEntry{
				TimestampBegin: from,
				TimestampEnd:   newTimestampEnd,
				OrderType:      "noOrder",
			}
			processedOrders = append(processedOrders, fullRow)
		}

		if index > 0 { // if not the first entry, add a noShift

			previousDataPoint := orderArray[index-1]
			timestampBegin := previousDataPoint.EndTimestamp
			timestampEnd := dataPoint.BeginTimestamp

			if timestampBegin != timestampEnd { // timestampBegin == timestampEnd ahppens when a no order is already in the list.
				// TODO: Fix
				fullRow := datamodel.OrderEntry{
					TimestampBegin: timestampBegin,
					TimestampEnd:   timestampEnd,
					OrderType:      "noOrder",
				}
				processedOrders = append(processedOrders, fullRow)
			}
		}

		// add original order
		fullRow := datamodel.OrderEntry{
			TimestampBegin: dataPoint.BeginTimestamp,
			TimestampEnd:   dataPoint.EndTimestamp,
			OrderType:      dataPoint.OrderName,
		}
		processedOrders = append(processedOrders, fullRow)

		// if last entry and previous order has finished, add a no order
		if index == len(orderArray)-1 && dataPoint.EndTimestamp.Before(to) {

			newTimestampBegin := dataPoint.EndTimestamp.Add(time.Duration(1) * time.Millisecond) // start one Millisecond after the previous order ended

			fullRow := datamodel.OrderEntry{
				TimestampBegin: newTimestampBegin,
				TimestampEnd:   to,
				OrderType:      "noOrder",
			}
			processedOrders = append(processedOrders, fullRow)
		}
	}
	return
}

func CalculateOrderInformation(
	rawOrders []datamodel.OrdersRaw,
	countSlice []datamodel.CountEntry,
	assetID uint32,
	rawStates []datamodel.StateEntry,
	rawShifts []datamodel.ShiftEntry,
	configuration datamodel.EnterpriseConfiguration,
	location, asset string) (data datamodel.DataResponseAny, errReturn error) {

	data.ColumnNames = []string{
		"Order ID",
		"Product ID",
		"Begin",
		"End",
		"Target units",
		"Actual units",
		"Target duration in seconds",
		"Actual duration in seconds",
		"Target time per unit in seconds",
		"Actual time per unit in seconds",
		datamodel.ConvertStateToString(datamodel.ProducingAtFullSpeedState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.ProducingAtLowerThanFullSpeedState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.UnknownState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.UnspecifiedStopState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.MicrostopState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.InletJamState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.OutletJamState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.CongestionBypassState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.MaterialIssueOtherState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.ChangeoverState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.CleaningState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.EmptyingState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.SettingUpState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.OperatorNotAtMachineState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.OperatorBreakState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.NoShiftState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.NoOrderState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.EquipmentFailureState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.ExternalFailureState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.ExternalInterferenceState, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.PreventiveMaintenanceStop, configuration.LanguageCode),
		datamodel.ConvertStateToString(datamodel.TechnicalOtherStop, configuration.LanguageCode),
		"Asset",
	}

	for _, rawOrder := range rawOrders {
		from := rawOrder.BeginTimestamp
		to := rawOrder.EndTimestamp

		beginTimestampInMs := float64(from.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))
		endTimestampInMs := float64(to.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))

		targetDuration := int(float64(rawOrder.TargetUnits) / rawOrder.TimePerUnitInSeconds)
		actualDuration := int((endTimestampInMs - beginTimestampInMs) / 1000)

		actualUnits := GetProducedPiecesFromCountSlice(countSlice, from, to)

		actualTimePerUnit := 0
		if actualUnits > 0 {
			actualTimePerUnit = actualDuration / int(actualUnits)
		}

		processedStates, err := ProcessStatesOptimized(
			assetID,
			rawStates,
			rawShifts,
			countSlice,
			rawOrders,
			from,
			to,
			configuration)
		if err != nil {
			errReturn = err
			return
		}

		// data.ColumnNames = []string{"state", "duration"}
		stopParetos, err := CalculateStopParetos(processedStates, to, true, true, configuration)
		if err != nil {
			errReturn = err
			return
		}

		ProducingAtFullSpeedStateDuration := 0.0
		ProducingAtLowerThanFullSpeedStateDuration := 0.0
		UnknownStateDuration := 0.0
		UnspecifiedStopStateDuration := 0.0
		MicrostopStateDuration := 0.0
		InletJamStateDuration := 0.0
		OutletJamStateDuration := 0.0
		CongestionBypassStateDuration := 0.0
		MaterialIssueOtherStateDuration := 0.0
		ChangeoverStateDuration := 0.0
		CleaningStateDuration := 0.0
		EmptyingStateDuration := 0.0
		SettingUpStateDuration := 0.0
		OperatorNotAtMachineStateDuration := 0.0
		OperatorBreakStateDuration := 0.0
		NoShiftStateDuration := 0.0
		NoOrderStateDuration := 0.0
		EquipmentFailureStateDuration := 0.0
		ExternalFailureStateDuration := 0.0
		ExternalInterferenceStateDuration := 0.0
		PreventiveMaintenanceStopDuration := 0.0
		TechnicalOtherStopDuration := 0.0

		for _, pareto := range stopParetos {
			state, ok := pareto[0].(int)
			if !ok {
				continue
			}
			duration, ok := pareto[1].(float64)
			if !ok {
				continue
			}

			if datamodel.IsProducingFullSpeed(state) {
				ProducingAtFullSpeedStateDuration += duration
			} else if datamodel.IsProducingLowerThanFullSpeed(state) {
				ProducingAtLowerThanFullSpeedStateDuration += duration
			} else if datamodel.IsUnknown(state) {
				UnknownStateDuration += duration
			} else if datamodel.IsUnspecifiedStop(state) {
				UnspecifiedStopStateDuration += duration
			} else if datamodel.IsMicrostop(state) {
				MicrostopStateDuration += duration
			} else if datamodel.IsInletJam(state) {
				InletJamStateDuration += duration
			} else if datamodel.IsOutletJam(state) {
				OutletJamStateDuration += duration
			} else if datamodel.IsCongestionBypass(state) {
				CongestionBypassStateDuration += duration
			} else if datamodel.IsMaterialIssueOther(state) {
				MaterialIssueOtherStateDuration += duration
			} else if datamodel.IsChangeover(state) {
				ChangeoverStateDuration += duration
			} else if datamodel.IsCleaning(state) {
				CleaningStateDuration += duration
			} else if datamodel.IsEmptying(state) {
				EmptyingStateDuration += duration
			} else if datamodel.IsSettingUp(state) {
				SettingUpStateDuration += duration
			} else if datamodel.IsOperatorNotAtMachine(state) {
				OperatorNotAtMachineStateDuration += duration
			} else if datamodel.IsOperatorBreak(state) {
				OperatorBreakStateDuration += duration
			} else if datamodel.IsNoShift(state) {
				NoShiftStateDuration += duration
			} else if datamodel.IsNoOrder(state) {
				NoOrderStateDuration += duration
			} else if datamodel.IsEquipmentFailure(state) {
				EquipmentFailureStateDuration += duration
			} else if datamodel.IsExternalFailure(state) {
				ExternalFailureStateDuration += duration
			} else if datamodel.IsExternalInterference(state) {
				ExternalInterferenceStateDuration += duration
			} else if datamodel.IsPreventiveMaintenance(state) {
				PreventiveMaintenanceStopDuration += duration
			} else if datamodel.IsTechnicalOtherStop(state) {
				TechnicalOtherStopDuration += duration
			}

		}

		fullRow := []interface{}{
			rawOrder.OrderName,
			rawOrder.ProductName,
			beginTimestampInMs,
			endTimestampInMs,
			rawOrder.TargetUnits,
			actualUnits,
			targetDuration,
			actualDuration,
			rawOrder.TimePerUnitInSeconds,
			actualTimePerUnit,
			ProducingAtFullSpeedStateDuration,          // 0
			ProducingAtLowerThanFullSpeedStateDuration, // 1
			UnknownStateDuration,                       // 2
			UnspecifiedStopStateDuration,
			MicrostopStateDuration,
			InletJamStateDuration,
			OutletJamStateDuration,
			CongestionBypassStateDuration,
			MaterialIssueOtherStateDuration,
			ChangeoverStateDuration,
			CleaningStateDuration,
			EmptyingStateDuration,
			SettingUpStateDuration,
			OperatorNotAtMachineStateDuration,
			OperatorBreakStateDuration,
			NoShiftStateDuration,
			NoOrderStateDuration,
			EquipmentFailureStateDuration,
			ExternalFailureStateDuration,
			ExternalInterferenceStateDuration,
			PreventiveMaintenanceStopDuration,
			TechnicalOtherStopDuration,
			location + "-" + asset,
		}

		data.Datapoints = append(data.Datapoints, fullRow)
	}

	return
}

// ProcessStatesOptimized splits up arrays efficiently for better caching
func ProcessStatesOptimized(
	workCellId uint32,
	stateArray []datamodel.StateEntry,
	rawShifts []datamodel.ShiftEntry,
	countSlice []datamodel.CountEntry,
	orderArray []datamodel.OrdersRaw,
	from, to time.Time,
	configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry, err error) {

	var processedStatesTemp []datamodel.StateEntry

	for current := from; current != to; {

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value
			zap.S().Debugf("[processStatesOptimized] currentTo (%d) is after to (%d)", currentTo.Unix(), to.Unix())
			processedStatesTemp, err = ProcessStates(
				workCellId,
				stateArray,
				rawShifts,
				countSlice,
				orderArray,
				current,
				to,
				configuration)
			if err != nil {
				zap.S().Errorf("processStates failed", err)
				return
			}
			current = to
		} else { // otherwise, calculate for entire time range
			zap.S().Debugf("[processStatesOptimized] currentTo (%d) is before to (%d)", currentTo.Unix(), to.Unix())
			processedStatesTemp, err = ProcessStates(
				workCellId,
				stateArray,
				rawShifts,
				countSlice,
				orderArray,
				current,
				currentTo,
				configuration)
			if err != nil {
				zap.S().Errorf("processStates failed", err)

				return
			}

			current = currentTo
		}
		// only add it if there is a valid datapoint. do not add areas with no state times
		if processedStatesTemp != nil {
			processedStateArray = append(processedStateArray, processedStatesTemp...)
		}
	}

	// resolving issue #17 (States change depending on the zoom level during time ranges longer than a day)
	processedStateArray = CombineAdjacentStops(processedStateArray)

	// For testing

	if logData {
		loggingTimestamp := time.Now()
		internal.LogObject("processStatesOptimized", "stateArray", loggingTimestamp, stateArray)
		internal.LogObject("processStatesOptimized", "rawShifts", loggingTimestamp, rawShifts)
		internal.LogObject("processStatesOptimized", "countSlice", loggingTimestamp, countSlice)
		internal.LogObject("processStatesOptimized", "orderArray", loggingTimestamp, orderArray)
		internal.LogObject("processStatesOptimized", "from", loggingTimestamp, from)
		internal.LogObject("processStatesOptimized", "to", loggingTimestamp, to)
		internal.LogObject("processStatesOptimized", "configuration", loggingTimestamp, configuration)
		internal.LogObject("processStatesOptimized", "processedStateArray", loggingTimestamp, processedStateArray)
	}

	return
}

// ProcessStates is responsible for cleaning states (e.g. remove the same state if it is adjacent)
// and calculating new ones (e.g. microstops)
func ProcessStates(
	workCellId uint32,
	stateArray []datamodel.StateEntry,
	rawShifts []datamodel.ShiftEntry,
	countSlice []datamodel.CountEntry,
	orderArray []datamodel.OrdersRaw,
	from, to time.Time,
	configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry, err error) {

	key := fmt.Sprintf("processStates-%d-%s-%s-%s", workCellId, from, to, internal.AsHash(configuration))

	// Get from cache if possible
	var cacheHit bool
	processedStateArray, cacheHit = internal.GetProcessStatesFromCache(key)
	if cacheHit {
		// zap.S().Debugf("processStates CacheHit")

		return
	}

	// remove elements outside [from, to]
	processedStateArray = RemoveUnnecessaryElementsFromStateSlice(stateArray, from, to)
	countSlice = RemoveUnnecessaryElementsFromCountSlice(countSlice, from, to)
	orderArray = RemoveUnnecessaryElementsFromOrderArray(orderArray, from, to)

	processedStateArray = RemoveSmallRunningStates(processedStateArray, configuration)

	processedStateArray = CombineAdjacentStops(processedStateArray) // this is required, because due to removeSmallRunningStates, specifyUnknownStopsWithFollowingStopReason we have now various stops in a row. this causes microstops longer than defined threshold

	processedStateArray = RemoveSmallStopStates(processedStateArray, configuration)

	processedStateArray = CombineAdjacentStops(processedStateArray) // this is required, because due to removeSmallRunningStates, specifyUnknownStopsWithFollowingStopReason we have now various stops in a row. this causes microstops longer than defined threshold

	processedStateArray = AddNoShiftsToStates(rawShifts, processedStateArray, to)

	processedStateArray = SpecifyUnknownStopsWithFollowingStopReason(processedStateArray) // sometimes the operator presses the button in the middle of a stop. Without this the time till pressing the button would be unknown stop. With this solution the entire block would be that stop.

	processedStateArray = CombineAdjacentStops(processedStateArray) // this is required, because due to removeSmallRunningStates, specifyUnknownStopsWithFollowingStopReason we have now various stops in a row. this causes microstops longer than defined threshold

	processedStateArray = AddLowSpeedStates(workCellId, processedStateArray, countSlice, configuration)

	processedStateArray = AddUnknownMicrostops(processedStateArray, configuration)

	processedStateArray, err = AutomaticallyIdentifyChangeovers(processedStateArray, orderArray, to, configuration)
	if err != nil {
		zap.S().Errorf("automaticallyIdentifyChangeovers failed", err)
		return
	}

	processedStateArray = SpecifySmallNoShiftsAsBreaks(processedStateArray, configuration)

	// Store to cache
	internal.StoreProcessStatesToCache(key, processedStateArray)

	return
}

func _(states []datamodel.StateEntry) {
	// Loop through all datapoints
	for index, dataPoint := range states {
		if index+1 == len(states) {
			continue
		}
		followingDataPoint := states[index+1]
		if followingDataPoint.Timestamp.Before(dataPoint.Timestamp) {
			zap.S().Errorf(
				"Found unordered states",
				dataPoint.State,
				dataPoint.Timestamp,
				followingDataPoint.State,
				followingDataPoint.Timestamp)

			for _, dataPoint2 := range states {
				zap.S().Debugf("States ", dataPoint2.State, dataPoint2.Timestamp)
			}
		}
	}
}

func GetParetoArray(durationArray []float64, stateArray []int, includeRunning bool) (
	paretos []datamodel.ParetoEntry,
	err error) {

	totalDurationChannel := make(chan datamodel.ChannelResult)

	uniqueStateArray := internal.UniqueInt(stateArray)

	// Loop through all datapoints and start getTotalDurationForState
	for _, state := range uniqueStateArray {
		go GetTotalDurationForState(durationArray, stateArray, state, totalDurationChannel)
	}

	// get all results back
	for i := 0; i < len(uniqueStateArray); i++ {
		currentResult := <-totalDurationChannel
		if currentResult.Err != nil {
			zap.S().Errorw(
				"Error in calculateDurations",
				"error", currentResult.Err,
			)
			err = currentResult.Err
			return
		}
		paretoEntry, ok := currentResult.ReturnValue.(datamodel.ParetoEntry)
		if !ok {
			continue
		}

		if paretoEntry.Duration < 0 {
			zap.S().Errorw(
				"negative duration",
				"duration", paretoEntry.Duration,
				"state", paretoEntry.State,
			)
			err = errors.New("negative state duration")
			return
		}

		// Add it if it is not running
		if !datamodel.IsProducing(paretoEntry.State) {
			paretos = append(paretos, paretoEntry)
		} else if datamodel.IsProducing(paretoEntry.State) && includeRunning { // add it if includeRunning is true
			paretos = append(paretos, paretoEntry)
		}
	}

	// Order results
	sort.Slice(
		paretos, func(i, j int) bool {
			return paretos[i].Duration > paretos[j].Duration
		})

	return
}

// CalculateStopParetos calculates the paretos for a given []datamodel.StateEntry
func CalculateStopParetos(
	temporaryDatapoints []datamodel.StateEntry,
	to time.Time,
	includeRunning, keepStatesInteger bool,
	configuration datamodel.EnterpriseConfiguration) (data [][]interface{}, err error) {

	durationArrayChannel := make(chan datamodel.ChannelResult)
	stateArrayChannel := make(chan datamodel.ChannelResult)

	// Execute parallel functions
	go CalculateDurations(temporaryDatapoints, to, durationArrayChannel)
	go TransformToStateArray(temporaryDatapoints, stateArrayChannel)

	// Get result from calculateDurations
	durationArrayResult := <-durationArrayChannel
	if durationArrayResult.Err != nil {
		zap.S().Errorf("Error in calculateDurations", durationArrayResult.Err)
		err = durationArrayResult.Err
		return
	}
	durationArray, ok := durationArrayResult.ReturnValue.([]float64)
	if !ok {
		err = errors.New("durationArray is not of type []float64")
		return
	}

	// Get result from transformToStateArray
	stateArrayResult := <-stateArrayChannel
	if durationArrayResult.Err != nil {
		zap.S().Errorf("Error in transformToStateArray", stateArrayResult.Err)
		err = stateArrayResult.Err
		return
	}
	stateArray, ok := stateArrayResult.ReturnValue.([]int)
	if !ok {
		err = errors.New("stateArrayResult.ReturnValue.([]int) failed")
		return
	}

	paretoArray, err := GetParetoArray(durationArray, stateArray, includeRunning)
	if err != nil {
		zap.S().Errorf("Error in getParetoArray", err)

		return
	}

	// Loop through all datapoints and start getTotalDurationForState
	for _, pareto := range paretoArray {
		if keepStatesInteger {
			fullRow := []interface{}{pareto.State, pareto.Duration}
			data = append(data, fullRow)
		} else {
			fullRow := []interface{}{ConvertStateToString(pareto.State, configuration), pareto.Duration}
			data = append(data, fullRow)
		}

	}

	return
}

// CalculateStateHistogram calculates the histogram for a given []datamodel.StateEntry
func CalculateStateHistogram(
	temporaryDatapoints []datamodel.StateEntry,
	includeRunning, keepStatesInteger bool,
	configuration datamodel.EnterpriseConfiguration) (data [][]interface{}, err error) {

	var stateOccurances [datamodel.MaxState]int // All are initialized with 0

	for _, state := range temporaryDatapoints {
		if state.State >= len(stateOccurances) || state.State < 0 {
			zap.S().Errorf("Invalid state", state.State)
			err = fmt.Errorf("invalid state: %d", state.State)
			return
		}
		stateOccurances[state.State]++
	}

	// Loop through all datapoints and start getTotalDurationForState
	for index, occurrences := range stateOccurances {
		if !includeRunning && index == 0 {
			continue
		}
		if occurrences == 0 { // only show elements where it happened at least once
			continue
		}
		if keepStatesInteger {
			fullRow := []interface{}{index, occurrences}
			data = append(data, fullRow)
		} else {
			fullRow := []interface{}{ConvertStateToString(index, configuration), occurrences}
			data = append(data, fullRow)
		}

	}

	return
}

// CalculateAvailability calculates the paretos for a given []ParetoDBResponse
func CalculateAvailability(
	temporaryDatapoints []datamodel.StateEntry,
	from, to time.Time,
	configuration datamodel.EnterpriseConfiguration) (data []interface{}, err error) {

	durationArrayChannel := make(chan datamodel.ChannelResult)
	stateArrayChannel := make(chan datamodel.ChannelResult)

	// Execute parallel functions
	go CalculateDurations(temporaryDatapoints, to, durationArrayChannel)
	go TransformToStateArray(temporaryDatapoints, stateArrayChannel)

	// Get result from calculateDurations
	durationArrayResult := <-durationArrayChannel
	if durationArrayResult.Err != nil {
		zap.S().Errorf("Error in calculateDurations", durationArrayResult.Err)
		err = durationArrayResult.Err
		return
	}
	durationArray, ok := durationArrayResult.ReturnValue.([]float64)
	if !ok {
		err = errors.New("durationArray is not of type []float64")
		return
	}

	// Get result from transformToStateArray
	stateArrayResult := <-stateArrayChannel
	if durationArrayResult.Err != nil {
		zap.S().Errorf("Error in transformToStateArray", stateArrayResult.Err)
		err = stateArrayResult.Err
		return
	}
	var stateArray []int
	stateArray, ok = stateArrayResult.ReturnValue.([]int)
	if !ok {
		err = errors.New("stateArrayResult.ReturnValue.([]int) failed")
		return
	}

	paretoArray, err := GetParetoArray(durationArray, stateArray, true)
	if err != nil {
		zap.S().Errorf("Error in getParetoArray", err)

		return
	}

	// Loop through all datapoints and calculate running and stop time
	timeRange := to.Sub(from).Seconds()

	var runningTime float64 = 0
	var stopTime float64 = 0
	var plannedTime float64 = timeRange // it starts with the full duration and then all idle times are subtracted from it

	for _, pareto := range paretoArray {
		if datamodel.IsProducingFullSpeed(pareto.State) {
			runningTime += pareto.Duration
		} else if IsAvailabilityLoss(int32(pareto.State), configuration) {
			stopTime += pareto.Duration
		} else if IsExcludedFromOEE(int32(pareto.State), configuration) {
			plannedTime -= pareto.Duration
		}
	}

	// Preventing NaN
	if plannedTime > 0 {
		data = []interface{}{(plannedTime - stopTime) / plannedTime, from}
	} else {
		data = nil
	}
	return
}

// CalculatePerformance calculates the paretos for a given []ParetoDBResponse
func CalculatePerformance(
	temporaryDatapoints []datamodel.StateEntry,
	from, to time.Time,
	configuration datamodel.EnterpriseConfiguration) (data []interface{}, err error) {

	durationArrayChannel := make(chan datamodel.ChannelResult)
	stateArrayChannel := make(chan datamodel.ChannelResult)

	// Execute parallel functions
	go CalculateDurations(temporaryDatapoints, to, durationArrayChannel)
	go TransformToStateArray(temporaryDatapoints, stateArrayChannel)

	// Get result from calculateDurations
	durationArrayResult := <-durationArrayChannel
	if durationArrayResult.Err != nil {
		zap.S().Errorf("Error in calculateDurations", durationArrayResult.Err)
		err = durationArrayResult.Err
		return
	}
	durationArray, ok := durationArrayResult.ReturnValue.([]float64)
	if !ok {
		err = errors.New("durationArrayResult.ReturnValue.([]float64) failed")
		return
	}

	// Get result from transformToStateArray
	stateArrayResult := <-stateArrayChannel
	if durationArrayResult.Err != nil {
		zap.S().Errorf("Error in transformToStateArray", stateArrayResult.Err)
		err = stateArrayResult.Err
		return
	}
	var stateArray []int
	stateArray, ok = stateArrayResult.ReturnValue.([]int)
	if !ok {
		err = errors.New("stateArrayResult.ReturnValue.([]int) failed")
		return
	}

	paretoArray, err := GetParetoArray(durationArray, stateArray, true)
	if err != nil {
		zap.S().Errorf("Error in getParetoArray", err)

		return
	}

	// Loop through all datapoints and calculate running and stop time
	var runningTime float64 = 0
	var stopTime float64 = 0

	for _, pareto := range paretoArray {
		if datamodel.IsProducingFullSpeed(pareto.State) {
			runningTime += pareto.Duration
		} else if IsPerformanceLoss(int32(pareto.State), configuration) {
			stopTime += pareto.Duration
		}
	}

	// TODO: add speed losses here

	// get all completed orders in timeframe and calculate speed loss by subtracting planned run time with actual run time. this is speedLossTime

	// final formula is: performance = runningTime / (runningTime + stopTime) - (runningTime - speedLossTime) / runningTime

	// also change this in OEE calculation

	// Preventing NaN
	if runningTime+stopTime > 0 {
		data = []interface{}{runningTime / (runningTime + stopTime), from}
	} else {
		data = nil
	}

	return
}

// CalculateQuality calculates the quality for a given []datamodel.CountEntry
func CalculateQuality(temporaryDatapoints []datamodel.CountEntry) (data []interface{}) {

	// Loop through all datapoints and calculate good pieces and scrap
	var total float64 = 0
	var scrap float64 = 0

	for _, currentCount := range temporaryDatapoints {
		total += currentCount.Count
		scrap += currentCount.Scrap
	}

	good := total - scrap

	// Preventing NaN
	if total > 0 {
		data = []interface{}{good / total}
	} else {
		data = nil
	}
	return
}

// IsPerformanceLoss checks whether a state is a performance loss as specified in configuration or derived from it
// (derived means it is not specifically mentioned in configuration, but the overarching category is)
func IsPerformanceLoss(state int32, configuration datamodel.EnterpriseConfiguration) (IsPerformanceLoss bool) {

	// Overarching categories are in the format 10000, 20000, 120000, etc.. We are checking if a value e.g. 20005 belongs to 20000
	quotient, _ := internal.Divmod(int64(state), 10000)

	if internal.IsInSliceInt32(configuration.PerformanceLossStates, int32(state)) { // Check if it is directly in it
		return true
	} else if !internal.IsInSliceInt32(
		configuration.AvailabilityLossStates,
		int32(state)) && internal.IsInSliceInt32(configuration.PerformanceLossStates, int32(quotient)) {
		// check whether it is not specifically in availability loss states.
		// If it is not mentioned there, check whether the overarching category is in it.
		return true
	}

	return
}

// IsAvailabilityLoss checks whether a state is an availability loss as specified in configuration or derived from it
// (derived means it is not specifically mentioned in configuration, but the overarching category is)
func IsAvailabilityLoss(state int32, configuration datamodel.EnterpriseConfiguration) (IsPerformanceLoss bool) {

	// Overarching categories are in the format 10000, 20000, 120000, etc.. We are checking if a value e.g. 20005 belongs to 20000
	quotient, _ := internal.Divmod(int64(state), 10000)

	if internal.IsInSliceInt32(configuration.AvailabilityLossStates, int32(state)) { // Check if it is directly in it
		return true
	} else if !internal.IsInSliceInt32(
		configuration.PerformanceLossStates,
		int32(state)) && internal.IsInSliceInt32(configuration.AvailabilityLossStates, int32(quotient)*10000) {
		// check whether it is not specifically in performance loss states.
		// If it is not mentioned there, check whether the overarching category is in it.
		return true
	}

	return
}

// IsExcludedFromOEE checks whether a state is neither a performance loss or availability loss, therefore, it needs to be taken out
// e.g., some companies remove noShift from the calculation
// A state is excluded, when it is neither in the performances nor in the availability bucket section in the configuration
func IsExcludedFromOEE(state int32, configuration datamodel.EnterpriseConfiguration) (IsExcluded bool) {

	// Overarching categories are in the format 10000, 20000, 120000, etc.. We are checking if a value e.g. 20005 belongs to 20000
	quotient, _ := internal.Divmod(int64(state), 10000)

	if internal.IsInSliceInt32(configuration.AvailabilityLossStates, int32(state)) { // Check if it is directly in it
		return false // if it is in availability, it is not excluded
	} else if internal.IsInSliceInt32(configuration.PerformanceLossStates, int32(state)) {
		return false // if it is in performance losses, then it cannot be excluded
	} else if internal.IsInSliceInt32(
		configuration.AvailabilityLossStates,
		int32(quotient)*10000) || internal.IsInSliceInt32(configuration.PerformanceLossStates, int32(quotient)*10000) {
		// if the overarching category in availability or performance loss states, then it cannot be excluded
		return false
	}
	// otherwise it is excluded
	return true
}

// CalculateOEE calculates the OEE
func CalculateOEE(
	temporaryDatapoints []datamodel.StateEntry,
	countSlice []datamodel.CountEntry,
	from, to time.Time,
	configuration datamodel.EnterpriseConfiguration) (data []interface{}, err error) {

	durationArrayChannel := make(chan datamodel.ChannelResult)
	stateArrayChannel := make(chan datamodel.ChannelResult)

	// Execute parallel functions
	go CalculateDurations(temporaryDatapoints, to, durationArrayChannel)
	go TransformToStateArray(temporaryDatapoints, stateArrayChannel)

	// Get result from calculateDurations
	durationArrayResult := <-durationArrayChannel
	if durationArrayResult.Err != nil {
		zap.S().Errorf("Error in calculateDurations", durationArrayResult.Err)
		err = durationArrayResult.Err
		return
	}
	durationArray, ok := durationArrayResult.ReturnValue.([]float64)
	if !ok {
		err = errors.New("could not convert return value to []float64")
		return
	}

	// Get result from transformToStateArray
	stateArrayResult := <-stateArrayChannel
	if durationArrayResult.Err != nil {
		zap.S().Errorf("Error in transformToStateArray", stateArrayResult.Err)
		err = stateArrayResult.Err
		return
	}
	var stateArray []int
	stateArray, ok = stateArrayResult.ReturnValue.([]int)
	if !ok {
		err = errors.New("could not convert return value to []int")
		return
	}

	paretoArray, err := GetParetoArray(durationArray, stateArray, true)
	if err != nil {
		zap.S().Errorf("Error in getParetoArray", err)

		return
	}

	// Loop through all datapoints and calculate running and stop time
	var runningTime float64 = 0
	var stopTime float64 = 0

	for _, pareto := range paretoArray {
		if datamodel.IsProducingFullSpeed(pareto.State) {
			runningTime = pareto.Duration
		} else if IsPerformanceLoss(int32(pareto.State), configuration) || IsAvailabilityLoss(
			int32(pareto.State),
			configuration) {
			stopTime += pareto.Duration
		}
	}

	// Calculate Quality
	quality := CalculateQuality(countSlice)
	var qualityRate float64
	// TODO: add speed losses here
	availabilityAndPerformanceRate := runningTime / (runningTime + stopTime)
	if len(quality) > 0 {
		qualityRate, ok = quality[0].(float64)
		if !ok {
			err = errors.New("could not convert quality to float64")
			return
		}
	} else {
		qualityRate = 1.0
	}
	finalOEE := availabilityAndPerformanceRate * qualityRate

	// Preventing NaN
	if runningTime+stopTime > 0 {
		data = []interface{}{finalOEE, from}
	} else {
		data = nil
	}

	return
}

// CalculateAverageStateTime calculates the average state time. It is used e.g. for calculating the average cleaning time.
func CalculateAverageStateTime(
	temporaryDatapoints []datamodel.StateEntry,
	from, to time.Time,
	configuration datamodel.EnterpriseConfiguration,
	targetState int) (data []interface{}, err error) {

	key := fmt.Sprintf(
		"CalculateAverageStateTime-%s-%s-%s-%s-%d",
		internal.AsHash(temporaryDatapoints),
		from,
		to,
		internal.AsHash(configuration),
		targetState)
	if database.Mutex.TryLock(key) { // is is already running?
		defer database.Mutex.Unlock(key)

		// Get from cache if possible
		var cacheHit bool
		data, cacheHit = internal.GetAverageStateTimeFromCache(key)
		if cacheHit { // data found
			zap.S().Debugf("CalculateAverageStateTime cache hit")
			return
		}

		var stateOccurances int
		var stateDurations float64

		for index, state := range temporaryDatapoints {

			if state.State != targetState {
				continue
			}

			// Step 1: increase occurrences
			stateOccurances++

			// Step 2: Calculate duration

			var timestampAfterCurrentOne time.Time

			// Special handling of last datapoint
			if index >= len(temporaryDatapoints)-1 {
				timestampAfterCurrentOne = to
			} else { // Get the following datapoint
				datapointAfterCurrentOne := temporaryDatapoints[index+1]
				timestampAfterCurrentOne = datapointAfterCurrentOne.Timestamp
			}

			timestampCurrent := state.Timestamp

			// additional error check (this fails if the states are not in order)
			if timestampAfterCurrentOne.Sub(timestampCurrent).Seconds() < 0 {
				err = errors.New("timestampAfterCurrentOne.Sub(timestampCurrent).Seconds() < 0 detected")
				BusinessLogicErrorHandling("calculateDurations", err, false)
				zap.S().Errorw(
					"timestampAfterCurrentOne.Sub(timestampCurrent).Seconds() < 0",
					"timestampAfterCurrentOne.Sub(timestampCurrent).Seconds()",
					timestampAfterCurrentOne.Sub(timestampCurrent).Seconds(),
					"timestampAfterCurrentOne",
					timestampAfterCurrentOne,
					"timestampCurrent",
					timestampCurrent,
					"state",
					state.State,
				)
			}

			duration := timestampAfterCurrentOne.Sub(timestampCurrent).Seconds()

			// Step 3: add to total duration
			stateDurations += duration
		}

		if stateOccurances != 0 {
			data = []interface{}{stateDurations / float64(stateOccurances), from}
		} else {
			data = nil
		}

		internal.StoreAverageStateTimeToCache(key, data)

	} else {
		zap.S().Errorf("Failed to get Mutex")
	}

	return
}

// getOrdersThatOverlapWithState gets all orders that overlap with a given time range (ignoring noOrders)
// this assumes that orderArray is in ascending order
func GetOrdersThatOverlapWithTimeRange(
	stateTimeRange TimeRange,
	orderArray []datamodel.OrdersRaw) (overlappingOrders []datamodel.OrdersRaw) {
	for _, order := range orderArray {

		if order.OrderName == "noOrder" { // only process proper orders and not the filler in between them
			continue
		}

		// if the order is entirely in TimeRange
		if IsTimepointInTimerange(order.BeginTimestamp, stateTimeRange) && IsTimepointInTimerange(
			order.EndTimestamp,
			stateTimeRange) {
			// this means the order is entirely in an unspecified state
			// ignoring

			continue
		}

		// if the order overlaps somehow, add it to overlapping orders
		if IsTimepointInTimerange(order.BeginTimestamp, stateTimeRange) || IsTimepointInTimerange(
			order.EndTimestamp,
			stateTimeRange) {
			overlappingOrders = append(overlappingOrders, order)

			continue
		}

	}
	return
}

// CalculateChangeoverStates splits up an unspecified stop into changeovers (assuming they are in order and there are no noOrder)
func CalculateChangeoverStates(
	stateTimeRange TimeRange,
	overlappingOrders []datamodel.OrdersRaw) (processedStateArray []datamodel.StateEntry, err error) {

	if len(overlappingOrders) == 1 {
		order := overlappingOrders[0]

		// if the order begin is in the timerange
		if IsTimepointInTimerange(order.BeginTimestamp, stateTimeRange) {

			fullRow := datamodel.StateEntry{
				State:     datamodel.UnspecifiedStopState,
				Timestamp: stateTimeRange.Begin,
			}
			processedStateArray = append(processedStateArray, fullRow)

			// start preparation process
			fullRow = datamodel.StateEntry{
				State:     datamodel.ChangeoverPreparationState,
				Timestamp: order.BeginTimestamp,
			}
			processedStateArray = append(processedStateArray, fullRow)

			return // we can abort here as there is no logical case that there would be any order after this (it would cause atleast one order be in the entire unspecified state)

		} else if IsTimepointInTimerange(
			order.EndTimestamp,
			stateTimeRange) { // if the end timestamp is in the timerange

			// start postprocessing process
			fullRow := datamodel.StateEntry{
				State:     datamodel.ChangeoverPostprocessingState,
				Timestamp: stateTimeRange.Begin,
			}

			processedStateArray = append(processedStateArray, fullRow)

			// unspecified stop after here
			fullRow = datamodel.StateEntry{
				State:     datamodel.UnspecifiedStopState,
				Timestamp: order.EndTimestamp,
			}
			processedStateArray = append(processedStateArray, fullRow)
		}

	} else if len(overlappingOrders) == 2 { // there is only one case left: the state has one order ending in it and one starting
		firstOrder := overlappingOrders[0]
		secondOrder := overlappingOrders[1]

		// start postprocessing process
		fullRow := datamodel.StateEntry{
			State:     datamodel.ChangeoverPostprocessingState,
			Timestamp: stateTimeRange.Begin,
		}

		processedStateArray = append(processedStateArray, fullRow)

		// changeover after here
		fullRow = datamodel.StateEntry{
			State:     datamodel.UnspecifiedStopState,
			Timestamp: firstOrder.EndTimestamp,
		}
		processedStateArray = append(processedStateArray, fullRow)

		// start preparation process
		fullRow = datamodel.StateEntry{
			State:     datamodel.ChangeoverPreparationState,
			Timestamp: secondOrder.BeginTimestamp,
		}
		processedStateArray = append(processedStateArray, fullRow)

	} else {
		// not possible. throw error
		err = errors.New("more than 2 overlapping orders with one state")
		return

	}

	return
}

// AutomaticallyIdentifyChangeovers automatically identifies changeovers if the corresponding configuration is set. See docs for more information.
func AutomaticallyIdentifyChangeovers(
	stateArray []datamodel.StateEntry,
	orderArray []datamodel.OrdersRaw,
	to time.Time,
	configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry, err error) {

	// Loop through all datapoints
	for index, dataPoint := range stateArray {

		if configuration.AutomaticallyIdentifyChangeovers { // only execute when configuration is set

			var state int
			var timestamp time.Time

			var followingDataPoint datamodel.StateEntry
			var stateTimeRange TimeRange

			if !datamodel.IsUnspecifiedStop(dataPoint.State) { // if not unspecified, do not do anything
				fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
				processedStateArray = append(processedStateArray, fullRow)
				continue
			}

			if index == len(stateArray)-1 { // if last entry, use end timestamp instead of following datapoint
				stateTimeRange = TimeRange{dataPoint.Timestamp, to}
			} else {
				followingDataPoint = stateArray[index+1]
				stateTimeRange = TimeRange{dataPoint.Timestamp, followingDataPoint.Timestamp}
			}

			overlappingOrders := GetOrdersThatOverlapWithTimeRange(stateTimeRange, orderArray)

			if len(overlappingOrders) != 0 { // if it overlaps

				var rows []datamodel.StateEntry
				rows, err = CalculateChangeoverStates(stateTimeRange, overlappingOrders)
				if err != nil {
					zap.S().Errorf("calculatateChangeoverStates failed", err)
					return
				}
				// Add all states
				processedStateArray = append(processedStateArray, rows...)

			} else { // if it does not overlap
				state = dataPoint.State
				timestamp = dataPoint.Timestamp
				fullRow := datamodel.StateEntry{
					State:     state,
					Timestamp: timestamp,
				}
				processedStateArray = append(processedStateArray, fullRow)
			}
		} else {
			fullRow := datamodel.StateEntry{
				State:     dataPoint.State,
				Timestamp: dataPoint.Timestamp,
			}
			processedStateArray = append(processedStateArray, fullRow)
		}
	}

	return
}

// ConvertOldToNewStateEntryArray converts a []datamodel.StateEntry from the old datamodel to the new one
func ConvertOldToNewStateEntryArray(stateArray []datamodel.StateEntry) (resultStateArray []datamodel.StateEntry) {

	// Loop through all datapoints
	for _, dataPoint := range stateArray {

		fullRow := datamodel.StateEntry{
			State:     datamodel.ConvertOldToNew(dataPoint.State),
			Timestamp: dataPoint.Timestamp,
		}
		resultStateArray = append(resultStateArray, fullRow)
	}

	return
}

// ConvertNewToOldStateEntryArray converts a []datamodel.StateEntry from the new datamodel to the old one
func ConvertNewToOldStateEntryArray(stateArray []datamodel.StateEntry) (resultStateArray []datamodel.StateEntry) {

	// Loop through all datapoints
	for _, dataPoint := range stateArray {

		fullRow := datamodel.StateEntry{
			State:     datamodel.ConvertNewToOld(dataPoint.State),
			Timestamp: dataPoint.Timestamp,
		}
		resultStateArray = append(resultStateArray, fullRow)
	}

	return
}

func SliceContainsInt(slice [][]interface{}, number, column int) (Contains bool, Index int) {
	for index, a := range slice {
		numberFromSlice, ok := a[column].(int)
		if !ok {
			zap.S().Errorf("sliceContainsInt: casting numberFromSlice to int error", index)
		}
		if numberFromSlice == number {
			return true, index
		}
	}
	return false, 0
}

// ChangeOutputFormat tests, if inputColumnName is already in output format and adds name, if not
func ChangeOutputFormat(data [][]interface{}, columnNames []string, inputColumnName string) (
	dataOutput [][]interface{},
	columnNamesOutput []string,
	columnIndex int) {
	for i, name := range columnNames {
		if name == inputColumnName {
			return data, columnNames, i
		}
	}
	// inputColumnName not previously found in existing columnNames: add to output
	columnNames = append(columnNames, inputColumnName)
	for i, slice := range data {
		slice = LengthenSliceToFitNames(slice, columnNames)
		data[i] = slice
	}
	columnIndex = len(columnNames) - 1
	return data, columnNames, columnIndex
}

// LengthenSliceToFitNames receives an interface slice and checks if it is as long as the names slice, if not it adds nil entries.
func LengthenSliceToFitNames(slice []interface{}, names []string) (sliceOutput []interface{}) {
	lengthNames := len(names)
	lengthSlice := len(slice)
	if lengthSlice == lengthNames {
		return slice
	} else if lengthSlice < lengthNames {
		for len(slice) < lengthNames {
			slice = append(slice, nil)
		}
		return slice
	} else {
		zap.S().Errorf("lengthenSliceToFitNames: slice too long")
	}
	return slice
}

// CreateNewRowInData adds a Row to data specifically for uniqueProductsWithTags, and fills in nil, where no information is known yet.
func CreateNewRowInData(
	data [][]interface{},
	columnNames []string,
	indexColumn, UID int,
	AID string,
	timestampBegin time.Time,
	timestampEnd sql.NullTime,
	productID int,
	isScrap bool,
	valueName sql.NullString,
	value sql.NullFloat64) (dataOut [][]interface{}) {
	var fullRow []interface{}
	fullRow = append(fullRow, UID)
	fullRow = append(fullRow, AID)
	fullRow = append(fullRow, float64(timestampBegin.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	if timestampEnd.Valid {
		fullRow = append(
			fullRow,
			float64(timestampEnd.Time.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	} else {
		fullRow = append(fullRow, nil)
	}
	fullRow = append(fullRow, productID)
	fullRow = append(fullRow, isScrap)
	fullRow = LengthenSliceToFitNames(fullRow, columnNames)
	if valueName.Valid && value.Valid { // if a value is specified, add to data
		fullRow[indexColumn] = value.Float64
	}
	dataOut = append(data, fullRow)
	return
}

// CheckOutputDimensions checks, if the length of columnNames corresponds to the length of each row of data
func CheckOutputDimensions(data [][]interface{}, columnNames []string) (err error) {
	length := len(columnNames)
	for _, row := range data {
		if length != len(row) {
			err = errors.New("error: data row length not consistent with columnname length")
			zap.S().Errorf("CheckOutputDimensions: dimensions wrong")
		}
	}
	return
}

func CalculateAccumulatedProducts(
	to, observationStart, observationEnd time.Time,
	countMap []models.CountStruct,
	orderMap []models.OrderStruct,
	productCache map[int]models.ProductStruct) (data datamodel.DataResponseAny) {

	var datapoints datamodel.DataResponseAny
	datapoints.ColumnNames = []string{
		"Target Output",
		"Actual Output",
		"Actual Scrap",
		"timestamp",
		"Internal Order ID",
		"Ordered Units",
		"Predicted Output",
		"Predicted Scrap",
		"Predicted Target",
		"Target Output after Order End",
		"Actual Output after Order End",
		"Actual Scrap after Order End",
		"Actual Good Output",
		"Actual Good Output after Order End",
		"Predicted Good Output",
	}
	// Move below to dataprocessing
	colLen := len(datapoints.ColumnNames)

	// Scale stepping based on observation range
	observationHours := to.Sub(observationStart).Hours()
	observationDays := int64(observationHours / 24)
	stepping := int64(60000) // 60 sec resolution
	if observationHours > 24 {
		stepping *= observationDays
	}

	// Pre-allocate memory for datapoints
	dplen := ((observationEnd.UnixMilli() - observationStart.UnixMilli()) / stepping) * 3
	dplen = int64(math.Max(float64(dplen), 10))
	zap.S().Debugf("Allocation for %d datapoints", dplen)
	tmpDatapoints := make([][]interface{}, dplen)

	zap.S().Debugf("Stepping %d (%f -> %d)", stepping, observationHours, observationDays)

	// Create datapoint every steppint
	dataPointIndex := 0

	cts := 0
	scp := 0

	lastOrderID := 0
	lastOrderOverhead := int64(0)
	allOrderOverheads := int64(0)
	lastDataPointTargetOverhead := int64(0)

	// Regression data for products
	regressionDataP := internal.Xy{
		X: make([]float64, dplen+1),
		Y: make([]float64, dplen+1),
	}

	// Regression data for scraps
	regressionDataS := internal.Xy{
		X: make([]float64, dplen+1),
		Y: make([]float64, dplen+1),
	}

	// Regression data for targets
	regressionDataT := internal.Xy{
		X: make([]float64, dplen+1),
		Y: make([]float64, dplen+1),
	}

	var step1ObservationEnd int64
	// Step through our observation timeframe
	for i := observationStart.UnixMilli(); i < observationEnd.UnixMilli(); i += stepping {
		steppingEnd := i + stepping
		step1ObservationEnd = i

		counts := make([]models.CountStruct, 0)

		for _, count := range countMap {
			if count.Timestamp.UnixMilli() >= i && count.Timestamp.UnixMilli() < steppingEnd {
				counts = append(
					counts,
					models.CountStruct{Timestamp: count.Timestamp, Count: count.Count, Scrap: count.Scrap})
			}
		}

		var orderID int
		var productId int
		var targetUnits int
		var beginTimeStamp time.Time
		runningOrder := false

		insideOrders := make([]models.OrderStruct, 0)

		for _, order := range orderMap {
			zap.S().Debugf(
				"if %d < %d && ((%b && %d >= %d) || !%b)",
				order.BeginTimeStamp.UnixMilli(),
				i,
				order.EndTimeStamp.Valid,
				order.EndTimeStamp.Time.UnixMilli(),
				steppingEnd,
				order.EndTimeStamp.Valid)
			if order.BeginTimeStamp.UnixMilli() < i && ((order.EndTimeStamp.Valid && order.EndTimeStamp.Time.UnixMilli() >= steppingEnd) || !order.EndTimeStamp.Valid) {
				orderID = order.OrderID
				productId = order.ProductId
				targetUnits = order.TargetUnits
				beginTimeStamp = order.BeginTimeStamp
				runningOrder = true
			}

			if order.BeginTimeStamp.UnixMilli() >= i && order.EndTimeStamp.Valid && order.EndTimeStamp.Time.UnixMilli() < steppingEnd {
				zap.S().Debugf("Found inside order ! (%d)", order.OrderID)
				insideOrders = append(
					insideOrders, models.OrderStruct{
						OrderID:        order.OrderID,
						ProductId:      order.ProductId,
						TargetUnits:    order.TargetUnits,
						BeginTimeStamp: order.BeginTimeStamp,
						EndTimeStamp:   order.EndTimeStamp,
					})
			}
		}

		expectedProducedFromCurrentOrder := int64(0)

		if runningOrder {
			timeSinceStartInMilliSec := i - beginTimeStamp.UnixMilli()
			product, ok := productCache[productId]
			if !ok {
				zap.S().Fatalf("Product %d not found", productId)
			}

			expectedProducedFromCurrentOrder = timeSinceStartInMilliSec / int64(product.TimePerProductUnitInSec*1000)
		} else {
			zap.S().Debugf("No running order")
		}

		// Orders inside step
		for _, insideOrder := range insideOrders {
			timeSinceStartInMilliSec := i - insideOrder.BeginTimeStamp.UnixMilli()
			product, ok := productCache[insideOrder.ProductId]
			if !ok {
				zap.S().Fatalf("Product %d not found", productId)
			}

			expectedProducedFromCurrentOrder += timeSinceStartInMilliSec / int64(product.TimePerProductUnitInSec*1000)
		}

		for _, count := range counts {
			cts += count.Count
			scp += count.Scrap
		}

		tmpDatapoints[dataPointIndex] = make([]interface{}, colLen)
		// Should fix rounding errors
		expT := int64(0)
		if expectedProducedFromCurrentOrder+allOrderOverheads < lastDataPointTargetOverhead {
			expT = lastDataPointTargetOverhead
		} else {
			expT = expectedProducedFromCurrentOrder + allOrderOverheads
		}
		// Target Output
		tmpDatapoints[dataPointIndex][0] = expT
		// Actual Output
		tmpDatapoints[dataPointIndex][1] = cts
		// Actual Scrap
		tmpDatapoints[dataPointIndex][2] = scp
		// timestamp
		tmpDatapoints[dataPointIndex][3] = i
		// Internal Order ID
		tmpDatapoints[dataPointIndex][4] = orderID
		// Ordered Units
		tmpDatapoints[dataPointIndex][5] = targetUnits
		// Predicted Output
		tmpDatapoints[dataPointIndex][6] = nil
		// Predicted Scrap
		tmpDatapoints[dataPointIndex][7] = nil
		// Predicted Target
		tmpDatapoints[dataPointIndex][8] = nil
		// Target Output after Order End
		tmpDatapoints[dataPointIndex][9] = nil
		// Actual Output after Order End
		tmpDatapoints[dataPointIndex][10] = nil
		// Actual Scrap after Order End
		tmpDatapoints[dataPointIndex][11] = nil
		// Actual Good Output
		tmpDatapoints[dataPointIndex][12] = cts - scp
		// Actual Good Output after Order End
		tmpDatapoints[dataPointIndex][13] = nil
		// Predicted Good Output
		tmpDatapoints[dataPointIndex][14] = nil

		if lastOrderID != orderID {
			allOrderOverheads += lastOrderOverhead
		}

		dataPointIndex += 1
		lastOrderID = orderID
		lastOrderOverhead = expectedProducedFromCurrentOrder
		lastDataPointTargetOverhead = expectedProducedFromCurrentOrder + allOrderOverheads

		// Add current count, scrap & target to regression list
		regressionDataP.X = append(regressionDataP.X, float64(dataPointIndex))
		regressionDataP.Y = append(regressionDataP.Y, float64(cts))

		regressionDataS.X = append(regressionDataS.X, float64(dataPointIndex))
		regressionDataS.Y = append(regressionDataS.Y, float64(scp))

		regressionDataT.X = append(regressionDataT.X, float64(dataPointIndex))
		regressionDataT.Y = append(regressionDataT.Y, float64(expT))

	}

	datapoints.Datapoints = make([][]interface{}, 0, dplen)

	// Make sure that there are no nil entries in our datapoints
	for _, item := range tmpDatapoints {
		if item != nil {
			datapoints.Datapoints = append(datapoints.Datapoints, item)
		}
	}

	if AfterOrEqual(observationEnd, to) {
		zap.S().Debugf("%s is AfterOrEqualTo %s", observationEnd, to)
		// No need to predict if observationEnd is after to
		return datapoints
	}

	// Begin predictions
	// If there is no data to predict from, just abort
	if dataPointIndex <= 3 {
		return datapoints
	}
	zap.S().Debugf("Before predictions. dataPointIndex: %d", dataPointIndex)
	dataPointIndex += 1
	betaP, alphaP := stat.LinearRegression(regressionDataP.X, regressionDataP.Y, nil, false)
	betaS, alphaS := stat.LinearRegression(regressionDataS.X, regressionDataS.Y, nil, false)
	betaT, alphaT := stat.LinearRegression(regressionDataT.X, regressionDataT.Y, nil, false)

	firstPValue := alphaP*float64(dataPointIndex) + betaP
	offsetP := float64(cts) - firstPValue

	firstSValue := alphaS*float64(dataPointIndex) + betaS
	offsetS := float64(scp) - firstSValue

	firstTValue := alphaT*float64(dataPointIndex) + betaT
	offsetT := float64(lastDataPointTargetOverhead) - firstTValue

	for i := step1ObservationEnd + 1; i < to.UnixMilli(); i += stepping {
		steppingEnd := i + stepping
		zap.S().Debugf("Prediction Step %d", i)
		zap.S().Debugf("Stepping %d", stepping)
		zap.S().Debugf("SteppingEnd %d", steppingEnd)

		for _, count := range countMap {
			if count.Timestamp.UnixMilli() >= i && count.Timestamp.UnixMilli() < steppingEnd {
				zap.S().Debugf(
					"Found count in timerange %d <= %d < %d (cnt: %d)",
					i,
					count.Timestamp.UnixMilli(),
					steppingEnd,
					count.Count)
				cts += count.Count
				scp += count.Scrap

				regressionDataP.X = append(regressionDataP.X, float64(dataPointIndex))
				regressionDataP.Y = append(regressionDataP.Y, float64(cts))

				regressionDataS.X = append(regressionDataS.X, float64(dataPointIndex))
				regressionDataS.Y = append(regressionDataS.Y, float64(scp))

				betaP, alphaP = stat.LinearRegression(regressionDataP.X, regressionDataP.Y, nil, false)
				betaS, alphaS = stat.LinearRegression(regressionDataS.X, regressionDataS.Y, nil, false)

			}
		}

		pValue := alphaP*float64(dataPointIndex) + betaP
		sValue := alphaS*float64(dataPointIndex) + betaS
		tValue := alphaT*float64(dataPointIndex) + betaT

		sVO := sValue + offsetS
		pVO := pValue + offsetP

		v := make([]interface{}, colLen)
		// Target Output
		v[0] = nil
		// Actual Output
		v[1] = nil
		// Actual Scrap
		v[2] = nil
		// timestamp
		v[3] = i
		// Internal Order ID
		v[4] = nil
		// Ordered Units
		v[5] = nil
		// Predicted Output
		v[6] = pVO
		// Predicted Scrap
		v[7] = sVO
		// Predicted Target
		v[8] = tValue + offsetT
		// Target Output after Order End
		v[9] = lastDataPointTargetOverhead
		// Actual Output after Order End
		v[10] = cts
		// Actual Scrap after Order End
		v[11] = scp
		// Actual Good Output
		v[12] = nil
		// Actual Good Output after Order End
		v[13] = cts - scp
		// Predicted Good Output
		v[14] = pVO - sVO

		datapoints.Datapoints = append(datapoints.Datapoints, v)

		dataPointIndex += 1
	}

	zap.S().Debugf("After predictions dataPointIndex: %d", dataPointIndex)
	return datapoints
}

// SplitCountSlice returns a slice of counts with the time being between from and to
func SplitCountSlice(counts []datamodel.CountEntry, from, to time.Time) []datamodel.CountEntry {
	var result []datamodel.CountEntry
	for _, count := range counts {
		if count.Timestamp.UnixMilli() >= from.UnixMilli() && count.Timestamp.UnixMilli() < to.UnixMilli() {
			result = append(result, count)
		}
	}
	return result
}

// AfterOrEqual returns if t is after or equal to u
func AfterOrEqual(t time.Time, u time.Time) bool {
	return t.After(u) || t.Equal(u)
}
