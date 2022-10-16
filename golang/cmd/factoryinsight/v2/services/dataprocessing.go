package services

import (
	"errors"
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"go.uber.org/zap"
	"time"
)

// ProcessStatesOptimized splits up arrays efficiently for better caching
func ProcessStatesOptimized(workCellId uint32, stateArray []datamodel.StateEntry, rawShifts []datamodel.ShiftEntry, countSlice []datamodel.CountEntry, orderArray []datamodel.OrdersRaw, from, to time.Time, configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry, err error) {

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
func ProcessStates(workCellId uint32, stateArray []datamodel.StateEntry, rawShifts []datamodel.ShiftEntry, countSlice []datamodel.CountEntry, orderArray []datamodel.OrdersRaw, from, to time.Time, configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry, err error) {

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

func RemoveUnnecessaryElementsFromStateSlice(processedStatesRaw []datamodel.StateEntry, from, to time.Time) (processedStates []datamodel.StateEntry) {
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

func RemoveUnnecessaryElementsFromCountSlice(countSlice []datamodel.CountEntry, from, to time.Time) (processedCountSlice []datamodel.CountEntry) {
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

func RemoveUnnecessaryElementsFromOrderArray(orderArray []datamodel.OrdersRaw, from, to time.Time) (processedOrdersArray []datamodel.OrdersRaw) {
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

// CalculatateLowSpeedStates splits up a "Running" state into multiple states either "Running" or "LowSpeed"
// additionally it caches it results. See also cache.go
func CalculatateLowSpeedStates(workCellId uint32, countSlice []datamodel.CountEntry, from, to time.Time, configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry) {

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

// Note: workCellId is only used for caching
func AddLowSpeedStates(workCellId uint32, stateArray []datamodel.StateEntry, countSlice []datamodel.CountEntry, configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry) {

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
func AddNoOrdersBetweenOrders(orderArray []datamodel.OrdersRaw, from time.Time, to time.Time) (processedOrders []datamodel.OrderEntry) {

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

// getOrdersThatOverlapWithState gets all orders that overlap with a given time range (ignoring noOrders)
// this assumes that orderArray is in ascending order
func GetOrdersThatOverlapWithTimeRange(stateTimeRange TimeRange, orderArray []datamodel.OrdersRaw) (overlappingOrders []datamodel.OrdersRaw) {
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
func CalculateChangeoverStates(stateTimeRange TimeRange, overlappingOrders []datamodel.OrdersRaw) (processedStateArray []datamodel.StateEntry, err error) {

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
func AutomaticallyIdentifyChangeovers(stateArray []datamodel.StateEntry, orderArray []datamodel.OrdersRaw, to time.Time, configuration datamodel.EnterpriseConfiguration) (processedStateArray []datamodel.StateEntry, err error) {

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
