package main

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"

	"go.uber.org/zap"
)

var logData bool = false
var lock sync.Mutex

// ChannelResult returns the returnValue and a error code from a goroutine
type ChannelResult struct {
	err         error
	returnValue interface{}
}

// ConvertStateToString converts a state in integer format to a human readable string
func ConvertStateToString(c *gin.Context, state int, languageCode int, configuration datamodel.CustomerConfiguration) (stateString string) {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "ConvertStateToString", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	languageCode = configuration.LanguageCode

	stateString = datamodel.ConvertStateToString(state, languageCode)

	return
}

// BusinessLogicErrorHandling logs and handles errors during the business logic
func BusinessLogicErrorHandling(c *gin.Context, operationName string, err error, isCritical bool) {

	traceID := "Failed to get traceID"
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "BusinessLogicErrorHandling", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()

		span.SetAttributes(attribute.String("error", err.Error()))

		traceID = span.SpanContext().SpanID().String()

	}

	zap.S().Errorw("Error in business logic. ",
		"operation name", operationName,
		"error", err,
		"traceID", traceID,
	)
	if isCritical {
		ShutdownApplicationGraceful()
	}
}

// ConvertActivityToString converts a maintenance activity in integer format to a human readable string
func ConvertActivityToString(c *gin.Context, activity int, configuration datamodel.CustomerConfiguration) (activityString string) {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "ConvertActivityToString", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	languageCode := configuration.LanguageCode

	if languageCode == 0 {
		switch activity {
		case 0:
			activityString = "Inspektion"
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

// calculateDurations returns an array with the duration between the states.
func calculateDurations(c *gin.Context, temporaryDatapoints []datamodel.StateEntry, from time.Time, to time.Time, returnChannel chan ChannelResult) {
	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "calculateDurations", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	// Prepare ChannelResult
	var durations []float64
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
			BusinessLogicErrorHandling(c, "calculateDurations", err, false)
			zap.S().Errorw("timestampAfterCurrentOne.Sub(timestampCurrent).Seconds() < 0",
				"timestampAfterCurrentOne.Sub(timestampCurrent).Seconds()", timestampAfterCurrentOne.Sub(timestampCurrent).Seconds(),
				"timestampAfterCurrentOne", timestampAfterCurrentOne,
				"timestampCurrent", timestampCurrent,
				"state", datapoint.State,
			)
		}
		durations = append(durations, timestampAfterCurrentOne.Sub(timestampCurrent).Seconds())
	}

	// Send ChannelResult back
	var ChannelResult ChannelResult
	ChannelResult.err = err
	ChannelResult.returnValue = durations
	returnChannel <- ChannelResult
}

func transformToStateArray(c *gin.Context, temporaryDatapoints []datamodel.StateEntry, returnChannel chan ChannelResult) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "transformToStateArray", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	// Prepare ChannelResult
	var stateArray []int
	var error error

	// Loop through all datapoints
	for _, datapoint := range temporaryDatapoints {
		stateArray = append(stateArray, datapoint.State)
	}

	// Send ChannelResult back
	var ChannelResult ChannelResult
	ChannelResult.err = error
	ChannelResult.returnValue = stateArray
	returnChannel <- ChannelResult
}

func getTotalDurationForState(c *gin.Context, durationArray []float64, stateArray []int, state int, returnChannel chan ChannelResult) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "getTotalDurationForState", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	// Prepare ChannelResult
	var totalDuration float64
	var err error

	totalDuration = 0

	// Loop through all datapoints and sum up total duration
	for index, datapoint := range stateArray {
		if datapoint == state {
			totalDuration += durationArray[index]
			if durationArray[index] < 0 {
				err = fmt.Errorf("durationArray[index] < 0: %f", durationArray[index])
				BusinessLogicErrorHandling(c, "getTotalDurationForState", err, false)
			}
		}
	}

	var ParetoEntry datamodel.ParetoEntry
	ParetoEntry.Duration = totalDuration
	ParetoEntry.State = state

	// Send ChannelResult back
	var ChannelResult ChannelResult
	ChannelResult.err = err
	ChannelResult.returnValue = ParetoEntry
	returnChannel <- ChannelResult
}

func addUnknownMicrostops(c *gin.Context, stateArray []datamodel.StateEntry, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "addUnknownMicrostops", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

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

		if index == len(stateArray)-1 || index == 0 { //if last entry or first entry, ignore
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

		if stateDuration <= configuration.MicrostopDurationInSeconds && datamodel.IsUnspecifiedStop(dataPoint.State) { //if duration smaller than configured threshold AND unknown stop
			state = datamodel.MicrostopState // microstop
		} else {
			state = dataPoint.State
		}

		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

func getProducedPiecesFromCountSlice(countSlice []datamodel.CountEntry, from time.Time, to time.Time) (totalCount float64) {

	// Loop through all datapoints
	for _, dataPoint := range countSlice {
		var timestamp time.Time
		var count float64

		count = dataPoint.Count
		timestamp = dataPoint.Timestamp

		if isTimepointInTimerange(timestamp, TimeRange{from, to}) {
			totalCount += count
		}
	}
	return
}

// Usage: defer timeTrack(time.Now(), "getProducedPiecesFromCountSlice")
func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
}

func removeUnnecessaryElementsFromCountSlice(countSlice []datamodel.CountEntry, from time.Time, to time.Time) (processedCountSlice []datamodel.CountEntry) {
	// Loop through all datapoints
	for _, dataPoint := range countSlice {
		if isTimepointInTimerange(dataPoint.Timestamp, TimeRange{from, to}) {
			processedCountSlice = append(processedCountSlice, dataPoint)
		}
	}
	return
}

func removeUnnecessaryElementsFromOrderArray(orderArray []datamodel.OrdersRaw, from time.Time, to time.Time) (processedOrdersArray []datamodel.OrdersRaw) {
	// Loop through all datapoints
	for _, dataPoint := range orderArray {
		if isTimepointInTimerange(dataPoint.BeginTimestamp, TimeRange{from, to}) || isTimepointInTimerange(dataPoint.EndTimestamp, TimeRange{from, to}) {
			processedOrdersArray = append(processedOrdersArray, dataPoint)
		}
	}
	return
}

func removeUnnecessaryElementsFromStateSlice(processedStatesRaw []datamodel.StateEntry, from time.Time, to time.Time) (processedStates []datamodel.StateEntry) {
	firstSelectedTimestampIndex := -1
	// Loop through all datapoints
	for index, dataPoint := range processedStatesRaw {
		// if is state in range or equal to from or to time range
		if isTimepointInTimerange(dataPoint.Timestamp, TimeRange{from, to}) || dataPoint.Timestamp == from || dataPoint.Timestamp == to {

			if firstSelectedTimestampIndex == -1 { //remember the first selected element
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

			processedStates = append([]datamodel.StateEntry{newDataPoint}, processedStates...) // prepand = put it as first element. reference: https://medium.com/@tzuni_eh/go-append-prepend-item-into-slice-a4bf167eb7af
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

		// if nothing has been found so far, use the last element (reason: there is no state after f"rom")
		lastElement := processedStatesRaw[len(processedStatesRaw)-1] // last element in the row
		newDataPoint := datamodel.StateEntry{}
		newDataPoint.Timestamp = from
		newDataPoint.State = lastElement.State
		processedStates = append(processedStates, newDataPoint)

	}
	return
}

// calculatateLowSpeedStates splits up a "Running" state into multiple states either "Running" or "LowSpeed"
// additionally it caches it results. See also cache.go
func calculatateLowSpeedStates(c *gin.Context, assetID uint32, countSlice []datamodel.CountEntry, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "calculatateLowSpeedStates", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	// Get from cache if possible
	processedStateArray, cacheHit := internal.GetCalculatateLowSpeedStatesFromCache(from, to, assetID)
	if cacheHit {
		return
	}

	countSlice = removeUnnecessaryElementsFromCountSlice(countSlice, from, to) // remove unnecessary items (items outside of current state) to improve speed

	var lastState int

	lastState = -1

	oldD := from

	for d := from; d.After(to) == false; d = d.Add(time.Minute) { //timestamp is beginning of the state. d is current progress.
		if d == oldD { //if first entry
			continue
		}

		averageProductionSpeedPerMinute := getProducedPiecesFromCountSlice(countSlice, oldD, d)

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
	internal.StoreCalculatateLowSpeedStatesToCache(from, to, assetID, processedStateArray)

	return
}

// Note: assetID is only used for caching
func addLowSpeedStates(c *gin.Context, assetID uint32, stateArray []datamodel.StateEntry, countSlice []datamodel.CountEntry, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "addLowSpeedStates", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	// actual function start
	// TODO: neglecting all other states with additional information, e.g. 10556

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if !datamodel.IsProducing(dataPoint.State) { //if not running, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 { //if last entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]
		stateDuration := followingDataPoint.Timestamp.Sub(dataPoint.Timestamp).Minutes()

		timestamp = dataPoint.Timestamp

		averageProductionSpeedPerMinute := getProducedPiecesFromCountSlice(countSlice, timestamp, followingDataPoint.Timestamp) / stateDuration

		if averageProductionSpeedPerMinute < configuration.LowSpeedThresholdInPcsPerHour/60 {
			rows, err := calculatateLowSpeedStates(c, assetID, countSlice, timestamp, followingDataPoint.Timestamp, configuration)
			if err != nil {
				zap.S().Errorf("calculatateLowSpeedStates failed", err)
				error = err
				return
			}
			// Add all states
			for _, row := range rows {
				processedStateArray = append(processedStateArray, row)
			}

		} else {
			state = dataPoint.State
			fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
			processedStateArray = append(processedStateArray, fullRow)
		}

	}

	return
}

func specifySmallNoShiftsAsBreaks(c *gin.Context, stateArray []datamodel.StateEntry, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "specifySmallNoShiftsAsBreaks", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if !datamodel.IsNoShift(dataPoint.State) { //if not noShift, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 { //if last entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]

		stateDuration := followingDataPoint.Timestamp.Sub(dataPoint.Timestamp).Seconds()

		timestamp = dataPoint.Timestamp

		if stateDuration <= configuration.ThresholdForNoShiftsConsideredBreakInSeconds { //if duration smaller than configured threshold AND unknown stop
			state = datamodel.OperatorBreakState // Break
		} else {
			state = dataPoint.State
		}

		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

func removeSmallRunningStates(c *gin.Context, stateArray []datamodel.StateEntry, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "removeSmallRunningStates", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if !datamodel.IsProducing(dataPoint.State) { //if not running, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 || index == 0 { //if last entry or first entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]

		stateDuration := followingDataPoint.Timestamp.Sub(dataPoint.Timestamp).Seconds()

		timestamp = dataPoint.Timestamp
		state = datamodel.ProducingAtFullSpeedState

		if stateDuration <= configuration.MinimumRunningTimeInSeconds { //if duration smaller than configured threshold
			continue // do not add it
		}

		// otherwise, add the running time
		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

func removeSmallStopStates(c *gin.Context, stateArray []datamodel.StateEntry, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "removeSmallStopStates", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if datamodel.IsProducing(dataPoint.State) { //if running, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 || index == 0 { //if last entry or first entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		followingDataPoint := stateArray[index+1]

		stateDuration := followingDataPoint.Timestamp.Sub(dataPoint.Timestamp).Seconds()

		timestamp = dataPoint.Timestamp
		state = dataPoint.State

		if stateDuration <= configuration.IgnoreMicrostopUnderThisDurationInSeconds { //if duration smaller than configured threshold
			continue // do not add it
		}

		// otherwise, add the running time
		fullRow := datamodel.StateEntry{State: state, Timestamp: timestamp}
		processedStateArray = append(processedStateArray, fullRow)
	}

	return
}

func combineAdjacentStops(c *gin.Context, stateArray []datamodel.StateEntry, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "combineAdjacentStops", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if datamodel.IsProducing(dataPoint.State) { //if running, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == 0 { //if first entry, ignore
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}
		previousDataPoint := stateArray[index-1]

		// if the current stop is an unknown stop and the previous one is not running (unspecified or specified stop) and not noShift (or break)
		if datamodel.IsUnspecifiedStop(dataPoint.State) && !datamodel.IsProducing(previousDataPoint.State) && !datamodel.IsNoShift(previousDataPoint.State) && !datamodel.IsOperatorBreak(previousDataPoint.State) {
			continue // then don't add the current state (it gives no additional information). As a result we remove adjacent unknown stops
		}

		// if the state is the same state as the previous one, then dont add it. Theoratically not possible. Practically happened several times.
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

func specifyUnknownStopsWithFollowingStopReason(c *gin.Context, stateArray []datamodel.StateEntry, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "specifyUnknownStopsWithFollowingStopReason", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	// Loop through all datapoints
	for index, dataPoint := range stateArray {
		var state int
		var timestamp time.Time

		if datamodel.IsProducing(dataPoint.State) { //if running or no shift, do not do anything
			fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
			processedStateArray = append(processedStateArray, fullRow)
			continue
		}

		if index == len(stateArray)-1 { //if last entry, ignore
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
func addNoOrdersBetweenOrders(orderArray []datamodel.OrdersRaw, from time.Time, to time.Time) (processedOrders []datamodel.OrderEntry) {

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

		if index > 0 { //if not the first entry, add a noShift

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

// GetOrdersTimeline gets all orders for a specific asset in a timerange for a timeline
func GetOrdersTimeline(c *gin.Context, customerID string, location string, asset string, from time.Time, to time.Time) (data datamodel.DataResponseAny, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "GetOrdersTimeline", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()

		span.SetAttributes(attribute.String("customerID", customerID))
		span.SetAttributes(attribute.String("location", location))
		span.SetAttributes(attribute.String("asset", asset))
		span.SetAttributes(attribute.String("from", from.String()))
		span.SetAttributes(attribute.String("to", to.String()))

	}

	JSONColumnName := customerID + "-" + location + "-" + asset + "-" + "order"
	data.ColumnNames = []string{"timestamp", JSONColumnName}

	//configuration := getCustomerConfiguration(span, customerID, location, asset)

	rawOrders, err := GetOrdersRaw(c, customerID, location, asset, from, to)
	if err != nil {
		zap.S().Errorf("GetOrdersRaw failed", err)
		error = err
		return
	}

	processedOrders := addNoOrdersBetweenOrders(rawOrders, from, to)

	// Loop through all datapoints
	for _, dataPoint := range processedOrders {
		fullRow := []interface{}{float64(dataPoint.TimestampBegin.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))), dataPoint.OrderType}
		data.Datapoints = append(data.Datapoints, fullRow)
	}
	return

}

func calculateOrderInformation(c *gin.Context, rawOrders []datamodel.OrdersRaw, countSlice []datamodel.CountEntry, assetID uint32, rawStates []datamodel.StateEntry, rawShifts []datamodel.ShiftEntry, configuration datamodel.CustomerConfiguration, location string, asset string) (data datamodel.DataResponseAny, errReturn error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "calculateOrderInformation", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

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
		datamodel.ConvertStateToString(datamodel.ProducingAtFullSpeedState, 1),
		datamodel.ConvertStateToString(datamodel.ProducingAtLowerThanFullSpeedState, 1),
		datamodel.ConvertStateToString(datamodel.UnknownState, 1),
		datamodel.ConvertStateToString(datamodel.UnspecifiedStopState, 1),
		datamodel.ConvertStateToString(datamodel.MicrostopState, 1),
		datamodel.ConvertStateToString(datamodel.InletJamState, 1),
		datamodel.ConvertStateToString(datamodel.OutletJamState, 1),
		datamodel.ConvertStateToString(datamodel.CongestionBypassState, 1),
		datamodel.ConvertStateToString(datamodel.MaterialIssueOtherState, 1),
		datamodel.ConvertStateToString(datamodel.ChangeoverState, 1),
		datamodel.ConvertStateToString(datamodel.CleaningState, 1),
		datamodel.ConvertStateToString(datamodel.EmptyingState, 1),
		datamodel.ConvertStateToString(datamodel.SettingUpState, 1),
		datamodel.ConvertStateToString(datamodel.OperatorNotAtMachineState, 1),
		datamodel.ConvertStateToString(datamodel.OperatorBreakState, 1),
		datamodel.ConvertStateToString(datamodel.NoShiftState, 1),
		datamodel.ConvertStateToString(datamodel.NoOrderState, 1),
		datamodel.ConvertStateToString(datamodel.EquipmentFailureState, 1),
		datamodel.ConvertStateToString(datamodel.ExternalFailureState, 1),
		datamodel.ConvertStateToString(datamodel.ExternalInterferenceState, 1),
		datamodel.ConvertStateToString(datamodel.PreventiveMaintenanceStop, 1),
		datamodel.ConvertStateToString(datamodel.TechnicalOtherStop, 1),
		"Asset",
	}

	for _, rawOrder := range rawOrders {
		from := rawOrder.BeginTimestamp
		to := rawOrder.EndTimestamp

		beginTimestampInMs := float64(from.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))
		endTimestampInMs := float64(to.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond)))

		targetDuration := int(float64(rawOrder.TargetUnits) / rawOrder.TimePerUnitInSeconds)
		actualDuration := int((endTimestampInMs - beginTimestampInMs) / 1000)

		actualUnits := getProducedPiecesFromCountSlice(countSlice, from, to)

		actualTimePerUnit := 0
		if actualUnits > 0 {
			actualTimePerUnit = int(actualDuration / int(actualUnits))
		}

		processedStates, err := processStatesOptimized(c, assetID, rawStates, rawShifts, countSlice, rawOrders, from, to, configuration)
		if err != nil {
			errReturn = err
			return
		}

		// data.ColumnNames = []string{"state", "duration"}
		stopParetos, err := CalculateStopParetos(c, processedStates, from, to, true, true, configuration)
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
			state := pareto[0].(int)
			duration := pareto[1].(float64)

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

// processStatesOptimized splits up arrays efficiently for better caching
func processStatesOptimized(c *gin.Context, assetID uint32, stateArray []datamodel.StateEntry, rawShifts []datamodel.ShiftEntry, countSlice []datamodel.CountEntry, orderArray []datamodel.OrdersRaw, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, err error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "processStatesOptimized", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	var processedStatesTemp []datamodel.StateEntry

	for current := from; current != to; {

		currentTo := current.AddDate(0, 0, 1)

		if currentTo.After(to) { // if the next 24h is out of timerange, only calculate OEE till the last value
			processedStatesTemp, err = processStates(c, assetID, stateArray, rawShifts, countSlice, orderArray, current, to, configuration)
			if err != nil {
				zap.S().Errorf("processStates failed", err)
				return
			}
			current = to
		} else { //otherwise, calculate for entire time range
			processedStatesTemp, err = processStates(c, assetID, stateArray, rawShifts, countSlice, orderArray, current, currentTo, configuration)
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
	processedStateArray, err = combineAdjacentStops(c, processedStateArray, configuration)
	if err != nil {
		zap.S().Errorf("combineAdjacentStops failed", err)
		return
	}

	// For testing
	loggingTimestamp := time.Now()
	if logData {
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

// processStates is responsible for cleaning states (e.g. remove the same state if it is adjacent)
// and calculating new ones (e.g. microstops)
func processStates(c *gin.Context,
	assetID uint32,
	stateArray []datamodel.StateEntry,
	rawShifts []datamodel.ShiftEntry,
	countSlice []datamodel.CountEntry,
	orderArray []datamodel.OrdersRaw,
	from time.Time,
	to time.Time,
	configuration datamodel.CustomerConfiguration,
) (
	processedStateArray []datamodel.StateEntry,
	err error,
) {

	var span oteltrace.Span
	if c != nil {
		_, span = tracer.Start(c.Request.Context(), "processStates", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	key := fmt.Sprintf("processStates-%d-%s-%s-%s", assetID, from, to, internal.AsHash(configuration))

	// Get from cache if possible
	var cacheHit bool
	processedStateArray, cacheHit = internal.GetProcessStatesFromCache(key)
	if cacheHit {
		//zap.S().Debugf("processStates CacheHit")
		if span != nil {
			span.SetAttributes(attribute.Bool("CacheHit", true))
		}
		return
	}

	// remove elements outside from, to
	processedStateArray = removeUnnecessaryElementsFromStateSlice(stateArray, from, to)
	countSlice = removeUnnecessaryElementsFromCountSlice(countSlice, from, to)
	orderArray = removeUnnecessaryElementsFromOrderArray(orderArray, from, to)

	processedStateArray, err = removeSmallRunningStates(c, processedStateArray, configuration)
	if err != nil {
		zap.S().Errorf("removeSmallRunningStates failed", err)
		return
	}

	processedStateArray, err = combineAdjacentStops(c, processedStateArray, configuration) // this is required, because due to removeSmallRunningStates, specifyUnknownStopsWithFollowingStopReason we have now various stops in a row. this causes microstops longer than defined threshold
	if err != nil {
		zap.S().Errorf("combineAdjacentStops failed", err)
		return
	}

	processedStateArray, err = removeSmallStopStates(c, processedStateArray, configuration)
	if err != nil {
		zap.S().Errorf("removeSmallStopStates failed", err)
		return
	}

	processedStateArray, err = combineAdjacentStops(c, processedStateArray, configuration) // this is required, because due to removeSmallRunningStates, specifyUnknownStopsWithFollowingStopReason we have now various stops in a row. this causes microstops longer than defined threshold
	if err != nil {
		zap.S().Errorf("combineAdjacentStops failed", err)
		return
	}

	processedStateArray, err = addNoShiftsToStates(c, rawShifts, processedStateArray, from, to, configuration)
	if err != nil {
		zap.S().Errorf("addNoShiftsToStates failed", err)
		return
	}

	processedStateArray, err = specifyUnknownStopsWithFollowingStopReason(c, processedStateArray, configuration) //sometimes the operator presses the button in the middle of a stop. Without this the time till pressing the button would be unknown stop. With this solution the entire block would be that stop.
	if err != nil {
		zap.S().Errorf("specifyUnknownStopsWithFollowingStopReason failed", err)
		return
	}

	processedStateArray, err = combineAdjacentStops(c, processedStateArray, configuration) // this is required, because due to removeSmallRunningStates, specifyUnknownStopsWithFollowingStopReason we have now various stops in a row. this causes microstops longer than defined threshold
	if err != nil {
		zap.S().Errorf("combineAdjacentStops failed", err)
		return
	}

	processedStateArray, err = addLowSpeedStates(c, assetID, processedStateArray, countSlice, configuration)
	if err != nil {
		zap.S().Errorf("addLowSpeedStates failed", err)
		return
	}

	processedStateArray, err = addUnknownMicrostops(c, processedStateArray, configuration)
	if err != nil {
		zap.S().Errorf("addUnknownMicrostops failed", err)
		return
	}

	processedStateArray, err = automaticallyIdentifyChangeovers(c, processedStateArray, orderArray, from, to, configuration)
	if err != nil {
		zap.S().Errorf("automaticallyIdentifyChangeovers failed", err)
		return
	}

	processedStateArray, err = specifySmallNoShiftsAsBreaks(c, processedStateArray, configuration)
	if err != nil {
		zap.S().Errorf("specifySmallNoShiftsAsBreaks failed", err)
		return
	}

	// Store to cache
	internal.StoreProcessStatesToCache(key, processedStateArray)

	return
}

func debugCheckForUnorderedStates(states []datamodel.StateEntry) {
	// Loop through all datapoints
	for index, dataPoint := range states {
		if index+1 == len(states) {
			continue
		}
		followingDataPoint := states[index+1]
		if followingDataPoint.Timestamp.Before(dataPoint.Timestamp) {
			zap.S().Errorf("Found unordered states", dataPoint.State, dataPoint.Timestamp, followingDataPoint.State, followingDataPoint.Timestamp)

			for _, dataPoint2 := range states {
				zap.S().Debugf("States ", dataPoint2.State, dataPoint2.Timestamp)
			}
		}
	}
}

func getParetoArray(c *gin.Context, durationArray []float64, stateArray []int, includeRunning bool) (paretos []datamodel.ParetoEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "getParetoArray", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	totalDurationChannel := make(chan ChannelResult)

	uniqueStateArray := internal.UniqueInt(stateArray)

	// Loop through all datapoints and start getTotalDurationForState
	for _, state := range uniqueStateArray {
		go getTotalDurationForState(c, durationArray, stateArray, state, totalDurationChannel)
	}

	// get all results back
	for i := 0; i < len(uniqueStateArray); i++ {
		currentResult := <-totalDurationChannel
		if currentResult.err != nil {
			zap.S().Errorw("Error in calculateDurations",
				"error", currentResult.err,
			)
			error = currentResult.err
			return
		}
		paretoEntry := currentResult.returnValue.(datamodel.ParetoEntry)

		if paretoEntry.Duration < 0 {
			zap.S().Errorw("negative duration",
				"duration", paretoEntry.Duration,
				"state", paretoEntry.State,
			)
			error = errors.New("negative state duration")
			return
		}

		// Add it if it is not running
		if !datamodel.IsProducing(paretoEntry.State) {
			paretos = append(paretos, paretoEntry)
		} else if datamodel.IsProducing(paretoEntry.State) && includeRunning == true { // add it if includeRunning is true
			paretos = append(paretos, paretoEntry)
		}
	}

	// Order results
	sort.Slice(paretos, func(i, j int) bool {
		return paretos[i].Duration > paretos[j].Duration
	})

	return
}

// CalculateStopParetos calculates the paretos for a given []datamodel.StateEntry
func CalculateStopParetos(c *gin.Context, temporaryDatapoints []datamodel.StateEntry, from time.Time, to time.Time, includeRunning bool, keepStatesInteger bool, configuration datamodel.CustomerConfiguration) (data [][]interface{}, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "CalculateStopParetos", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	durationArrayChannel := make(chan ChannelResult)
	stateArrayChannel := make(chan ChannelResult)

	// Execute parallel functions
	go calculateDurations(c, temporaryDatapoints, from, to, durationArrayChannel)
	go transformToStateArray(c, temporaryDatapoints, stateArrayChannel)

	// Get result from calculateDurations
	durationArrayResult := <-durationArrayChannel
	if durationArrayResult.err != nil {
		zap.S().Errorf("Error in calculateDurations", durationArrayResult.err)
		error = durationArrayResult.err
		return
	}
	durationArray := durationArrayResult.returnValue.([]float64)

	// Get result from transformToStateArray
	stateArrayResult := <-stateArrayChannel
	if durationArrayResult.err != nil {
		zap.S().Errorf("Error in transformToStateArray", stateArrayResult.err)
		error = stateArrayResult.err
		return
	}
	stateArray := stateArrayResult.returnValue.([]int)

	paretoArray, err := getParetoArray(c, durationArray, stateArray, includeRunning)
	if err != nil {
		zap.S().Errorf("Error in getParetoArray", err)
		error = err
		return
	}

	// Loop through all datapoints and start getTotalDurationForState
	for _, pareto := range paretoArray {
		if keepStatesInteger {
			fullRow := []interface{}{pareto.State, pareto.Duration}
			data = append(data, fullRow)
		} else {
			fullRow := []interface{}{ConvertStateToString(c, pareto.State, 0, configuration), pareto.Duration}
			data = append(data, fullRow)
		}

	}

	return
}

// CalculateStateHistogram calculates the histogram for a given []datamodel.StateEntry
func CalculateStateHistogram(c *gin.Context, temporaryDatapoints []datamodel.StateEntry, from time.Time, to time.Time, includeRunning bool, keepStatesInteger bool, configuration datamodel.CustomerConfiguration) (data [][]interface{}, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "CalculateStateHistogram", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	var stateOccurances [datamodel.MaxState]int //All are initialized with 0

	for _, state := range temporaryDatapoints {
		if state.State >= len(stateOccurances) || state.State < 0 {
			zap.S().Errorf("Invalid state", state.State)
			error = fmt.Errorf("Invalid state: %d", state.State)
			return
		}
		stateOccurances[int(state.State)]++
	}

	// Loop through all datapoints and start getTotalDurationForState
	for index, occurances := range stateOccurances {
		if !includeRunning && index == 0 {
			continue
		}
		if occurances == 0 { // only show elements where it happened at least once
			continue
		}
		if keepStatesInteger {
			fullRow := []interface{}{index, occurances}
			data = append(data, fullRow)
		} else {
			fullRow := []interface{}{ConvertStateToString(c, index, 0, configuration), occurances}
			data = append(data, fullRow)
		}

	}

	return
}

// CalculateAvailability calculates the paretos for a given []ParetoDBResponse
func CalculateAvailability(c *gin.Context, temporaryDatapoints []datamodel.StateEntry, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (data [][]interface{}, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "CalculateAvailability", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}
	durationArrayChannel := make(chan ChannelResult)
	stateArrayChannel := make(chan ChannelResult)

	// Execute parallel functions
	go calculateDurations(c, temporaryDatapoints, from, to, durationArrayChannel)
	go transformToStateArray(c, temporaryDatapoints, stateArrayChannel)

	// Get result from calculateDurations
	durationArrayResult := <-durationArrayChannel
	if durationArrayResult.err != nil {
		zap.S().Errorf("Error in calculateDurations", durationArrayResult.err)
		error = durationArrayResult.err
		return
	}
	durationArray := durationArrayResult.returnValue.([]float64)

	// Get result from transformToStateArray
	stateArrayResult := <-stateArrayChannel
	if durationArrayResult.err != nil {
		zap.S().Errorf("Error in transformToStateArray", stateArrayResult.err)
		error = stateArrayResult.err
		return
	}
	stateArray := stateArrayResult.returnValue.([]int)

	paretoArray, err := getParetoArray(c, durationArray, stateArray, true)
	if err != nil {
		zap.S().Errorf("Error in getParetoArray", err)
		error = err
		return
	}

	// Loop through all datapoints and calculate running and stop time
	var runningTime float64 = 0
	var stopTime float64 = 0

	for _, pareto := range paretoArray {
		if datamodel.IsProducingFullSpeed(pareto.State) {
			runningTime = pareto.Duration
		} else if IsAvailabilityLoss(int32(pareto.State), configuration) {
			stopTime += pareto.Duration
		}
	}

	fullRow := []interface{}{runningTime / (runningTime + stopTime)}
	data = append(data, fullRow)

	return
}

// CalculatePerformance calculates the paretos for a given []ParetoDBResponse
func CalculatePerformance(c *gin.Context, temporaryDatapoints []datamodel.StateEntry, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (data [][]interface{}, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "CalculatePerformance", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	durationArrayChannel := make(chan ChannelResult)
	stateArrayChannel := make(chan ChannelResult)

	// Execute parallel functions
	go calculateDurations(c, temporaryDatapoints, from, to, durationArrayChannel)
	go transformToStateArray(c, temporaryDatapoints, stateArrayChannel)

	// Get result from calculateDurations
	durationArrayResult := <-durationArrayChannel
	if durationArrayResult.err != nil {
		zap.S().Errorf("Error in calculateDurations", durationArrayResult.err)
		error = durationArrayResult.err
		return
	}
	durationArray := durationArrayResult.returnValue.([]float64)

	// Get result from transformToStateArray
	stateArrayResult := <-stateArrayChannel
	if durationArrayResult.err != nil {
		zap.S().Errorf("Error in transformToStateArray", stateArrayResult.err)
		error = stateArrayResult.err
		return
	}
	stateArray := stateArrayResult.returnValue.([]int)

	paretoArray, err := getParetoArray(c, durationArray, stateArray, true)
	if err != nil {
		zap.S().Errorf("Error in getParetoArray", err)
		error = err
		return
	}

	// Loop through all datapoints and calculate running and stop time
	var runningTime float64 = 0
	var stopTime float64 = 0

	for _, pareto := range paretoArray {
		if datamodel.IsProducingFullSpeed(pareto.State) {
			runningTime = pareto.Duration
		} else if IsPerformanceLoss(int32(pareto.State), configuration) {
			stopTime += pareto.Duration
		}
	}

	fullRow := []interface{}{runningTime / (runningTime + stopTime)}
	data = append(data, fullRow)

	return
}

// CalculateQuality calculates the quality for a given []datamodel.CountEntry
func CalculateQuality(c *gin.Context, temporaryDatapoints []datamodel.CountEntry, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (data [][]interface{}, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "CalculatePerformance", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	// Loop through all datapoints and calculate good pieces and scrap
	var total float64 = 0
	var scrap float64 = 0

	for _, currentCount := range temporaryDatapoints {
		total += currentCount.Count
		scrap += currentCount.Scrap
	}

	good := total - scrap

	fullRow := []interface{}{good / total}
	data = append(data, fullRow)

	return
}

// IsPerformanceLoss checks whether a state is a performance loss as specified in configuration or derived from it
// (derived means it is not specifically mentioned in configuration, but the overarching category is)
func IsPerformanceLoss(state int32, configuration datamodel.CustomerConfiguration) (IsPerformanceLoss bool) {

	// Overarching categories are in the format 10000, 20000, 120000, etc.. We are checking if a value e.g. 20005 belongs to 20000
	quotient, _ := internal.Divmod(int64(state), 10000)

	if internal.IsInSliceInt32(configuration.PerformanceLossStates, int32(state)) { // Check if it is directly in it
		return true
	} else if !internal.IsInSliceInt32(configuration.AvailabilityLossStates, int32(state)) && internal.IsInSliceInt32(configuration.PerformanceLossStates, int32(quotient)) {
		// check whether it is not specifically in availability loss states.
		// If it is not mentioned htere, check whether the overarching category is in it.
		return true
	}

	return
}

// IsAvailabilityLoss checks whether a state is a availability loss as specified in configuration or derived from it
// (derived means it is not specifically mentioned in configuration, but the overarching category is)
func IsAvailabilityLoss(state int32, configuration datamodel.CustomerConfiguration) (IsPerformanceLoss bool) {

	// Overarching categories are in the format 10000, 20000, 120000, etc.. We are checking if a value e.g. 20005 belongs to 20000
	quotient, _ := internal.Divmod(int64(state), 10000)

	if internal.IsInSliceInt32(configuration.AvailabilityLossStates, int32(state)) { // Check if it is directly in it
		return true
	} else if !internal.IsInSliceInt32(configuration.PerformanceLossStates, int32(state)) && internal.IsInSliceInt32(configuration.AvailabilityLossStates, int32(quotient)*10000) {
		// check whether it is not specifically in performance loss states.
		// If it is not mentioned htere, check whether the overarching category is in it.
		return true
	}

	return
}

// CalculateOEE calculates the OEE
func CalculateOEE(c *gin.Context, temporaryDatapoints []datamodel.StateEntry, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (data []interface{}, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "CalculateOEE", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	durationArrayChannel := make(chan ChannelResult)
	stateArrayChannel := make(chan ChannelResult)

	// Execute parallel functions
	go calculateDurations(c, temporaryDatapoints, from, to, durationArrayChannel)
	go transformToStateArray(c, temporaryDatapoints, stateArrayChannel)

	// Get result from calculateDurations
	durationArrayResult := <-durationArrayChannel
	if durationArrayResult.err != nil {
		zap.S().Errorf("Error in calculateDurations", durationArrayResult.err)
		error = durationArrayResult.err
		return
	}
	durationArray := durationArrayResult.returnValue.([]float64)

	// Get result from transformToStateArray
	stateArrayResult := <-stateArrayChannel
	if durationArrayResult.err != nil {
		zap.S().Errorf("Error in transformToStateArray", stateArrayResult.err)
		error = stateArrayResult.err
		return
	}
	stateArray := stateArrayResult.returnValue.([]int)

	paretoArray, err := getParetoArray(c, durationArray, stateArray, true)
	if err != nil {
		zap.S().Errorf("Error in getParetoArray", err)
		error = err
		return
	}

	// Loop through all datapoints and calculate running and stop time
	var runningTime float64 = 0
	var stopTime float64 = 0

	for _, pareto := range paretoArray {
		if datamodel.IsProducingFullSpeed(pareto.State) {
			runningTime = pareto.Duration
		} else if IsPerformanceLoss(int32(pareto.State), configuration) || IsAvailabilityLoss(int32(pareto.State), configuration) {
			stopTime += pareto.Duration
		}
	}

	// Preventing NaN
	if runningTime+stopTime > 0 {
		data = []interface{}{runningTime / (runningTime + stopTime), from}
	} else {
		data = nil
	}

	return
}

// CalculateAverageStateTime calculates the average state time. It is used e.g. for calculating the average cleaning time.
func CalculateAverageStateTime(c *gin.Context, temporaryDatapoints []datamodel.StateEntry, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration, targetState int) (data []interface{}, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "CalculateAverageStateTime", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	key := fmt.Sprintf("CalculateAverageStateTime-%s-%s-%s-%s-%d", internal.AsHash(temporaryDatapoints), from, to, internal.AsHash(configuration), targetState)
	if mutex.TryLock(key) { // is is already running?
		defer mutex.Unlock(key)

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

			// Step 1: increase occurances
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
				error = errors.New("timestampAfterCurrentOne.Sub(timestampCurrent).Seconds() < 0 detected")
				BusinessLogicErrorHandling(c, "calculateDurations", error, false)
				zap.S().Errorw("timestampAfterCurrentOne.Sub(timestampCurrent).Seconds() < 0",
					"timestampAfterCurrentOne.Sub(timestampCurrent).Seconds()", timestampAfterCurrentOne.Sub(timestampCurrent).Seconds(),
					"timestampAfterCurrentOne", timestampAfterCurrentOne,
					"timestampCurrent", timestampCurrent,
					"state", state.State,
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
func getOrdersThatOverlapWithTimeRange(stateTimeRange TimeRange, orderArray []datamodel.OrdersRaw) (overlappingOrders []datamodel.OrdersRaw) {
	for _, order := range orderArray {

		if order.OrderName == "noOrder" { // only process proper orders and not the filler in between them
			continue
		}

		// if the order is entirely in TimeRange
		if isTimepointInTimerange(order.BeginTimestamp, stateTimeRange) && isTimepointInTimerange(order.EndTimestamp, stateTimeRange) {
			// this means the order is entirely in an unspecified state
			// ignoring

			continue
		}

		// if the order overlaps somehow, add it to overlapping orders
		if isTimepointInTimerange(order.BeginTimestamp, stateTimeRange) || isTimepointInTimerange(order.EndTimestamp, stateTimeRange) {
			overlappingOrders = append(overlappingOrders, order)

			continue
		}

	}
	return
}

// calculateChangeoverStates splits up an unspecified stop into changeovers (assuming they are in order and there are no noOrder)
func calculateChangeoverStates(stateTimeRange TimeRange, overlappingOrders []datamodel.OrdersRaw) (processedStateArray []datamodel.StateEntry, error error) {

	if len(overlappingOrders) == 1 {
		order := overlappingOrders[0]

		// if the order begin is in the timerange
		if isTimepointInTimerange(order.BeginTimestamp, stateTimeRange) {

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

		} else if isTimepointInTimerange(order.EndTimestamp, stateTimeRange) { // if the end timestamp is in the timerange

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
		error = errors.New("More than 2 overlapping orders with one state")
		return

	}

	return
}

// automaticallyIdentifyChangeovers automatically identifies changeovers if the corresponding configuration is set. See docs for more information.
func automaticallyIdentifyChangeovers(c *gin.Context, stateArray []datamodel.StateEntry, orderArray []datamodel.OrdersRaw, from time.Time, to time.Time, configuration datamodel.CustomerConfiguration) (processedStateArray []datamodel.StateEntry, error error) {

	if c != nil {
		_, span := tracer.Start(c.Request.Context(), "automaticallyIdentifyChangeovers", oteltrace.WithAttributes(attribute.String("method", c.Request.Method), attribute.String("path", c.Request.URL.Path)))
		defer span.End()
	}

	// Loop through all datapoints
	for index, dataPoint := range stateArray {

		if configuration.AutomaticallyIdentifyChangeovers { // only execute when configuration is set

			var state int
			var timestamp time.Time

			var followingDataPoint datamodel.StateEntry
			var stateTimeRange TimeRange

			if !datamodel.IsUnspecifiedStop(dataPoint.State) { //if not unspecified, do not do anything
				fullRow := datamodel.StateEntry{State: dataPoint.State, Timestamp: dataPoint.Timestamp}
				processedStateArray = append(processedStateArray, fullRow)
				continue
			}

			if index == len(stateArray)-1 { //if last entry, use end timestamp instead of following datapoint
				stateTimeRange = TimeRange{dataPoint.Timestamp, to}
			} else {
				followingDataPoint = stateArray[index+1]
				stateTimeRange = TimeRange{dataPoint.Timestamp, followingDataPoint.Timestamp}
			}

			overlappingOrders := getOrdersThatOverlapWithTimeRange(stateTimeRange, orderArray)

			if len(overlappingOrders) != 0 { // if it overlaps

				rows, err := calculateChangeoverStates(stateTimeRange, overlappingOrders)
				if err != nil {
					zap.S().Errorf("calculatateChangeoverStates failed", err)
					error = err
					return
				}
				// Add all states
				for _, row := range rows {
					processedStateArray = append(processedStateArray, row)
				}

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

func SliceContainsInt(slice [][]interface{}, number int, column int) (Contains bool, Index int) {
	for index, a := range slice {
		numberFromSlice, ok := a[column].(int)
		if ok == false {
			zap.S().Errorf("sliceContainsInt: casting numberFromSlice to int error", index)
		}
		if numberFromSlice == number {
			return true, index
		}
	}
	return false, 0
}

//ChangeOutputFormat tests, if inputColumnName is already in output format and adds name, if not
func ChangeOutputFormat(data [][]interface{}, columnNames []string, inputColumnName string) (dataOutput [][]interface{},
	columnNamesOutput []string, columnIndex int) {
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

//LengthenSliceToFitNames receives an interface slice and checks if it is as long as the names slice, if not it adds nil entries.
func LengthenSliceToFitNames(slice []interface{}, names []string) (sliceOutput []interface{}) {
	lengthNames := len(names)
	if len(slice) == lengthNames {
		return slice
	} else if len(slice) < lengthNames {
		for len(slice) < lengthNames {
			slice = append(slice, nil)
		}
		return slice
	} else {
		zap.S().Errorf("lengthenSliceToFitNames: slice too long")
	}
	return slice
}

//CreateNewRowInData adds a Row to data specifically for uniqueProductsWithTags, and fills in nil, where no information is known yet.
func CreateNewRowInData(data [][]interface{}, columnNames []string, indexColumn int, UID int, AID string,
	timestampBegin time.Time, timestampEnd sql.NullTime, productID int, isScrap bool, valueName sql.NullString,
	value sql.NullFloat64) (dataOut [][]interface{}) {
	var fullRow []interface{}
	fullRow = append(fullRow, UID)
	fullRow = append(fullRow, AID)
	fullRow = append(fullRow, float64(timestampBegin.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	if timestampEnd.Valid {
		fullRow = append(fullRow, float64(timestampEnd.Time.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	} else {
		fullRow = append(fullRow, nil)
	}
	fullRow = append(fullRow, productID)
	fullRow = append(fullRow, isScrap)
	fullRow = LengthenSliceToFitNames(fullRow, columnNames)
	if valueName.Valid == true && value.Valid == true { //if a value is specified, add to data
		fullRow[indexColumn] = value.Float64
	}
	dataOut = append(data, fullRow)
	return
}

//CheckOutputDimensions checks, if the length of columnNames corresponds to the length of each row of data
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
