package repository

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"time"
)

// ConvertStateToString converts a state in integer format to a human-readable string
func ConvertStateToString(state int, configuration datamodel.EnterpriseConfiguration) (stateString string) {

	languageCode := configuration.LanguageCode

	stateString = datamodel.ConvertStateToString(state, languageCode)

	return
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

func TransformToStateArray(temporaryDatapoints []datamodel.StateEntry, returnChannel chan datamodel.ChannelResult) {

	// Prepare ChannelResult
	stateArray := make([]int, 0, len(temporaryDatapoints))
	var err error

	// Loop through all datapoints
	for _, datapoint := range temporaryDatapoints {
		stateArray = append(stateArray, datapoint.State)
	}

	// Send ChannelResult back
	var ChannelResultInstance datamodel.ChannelResult
	ChannelResultInstance.Err = err
	ChannelResultInstance.ReturnValue = stateArray
	returnChannel <- ChannelResultInstance
}

func GetParetoArray(durationArray []float64, stateArray []int, includeRunning bool) (
	// TODO: implement me

	paretos []datamodel.ParetoEntry,
	err error) {
	/*
		totalDurationChannel := make(chan datamodel.ChannelResult)

		uniqueStateArray := internal.UniqueInt(stateArray)

		// Loop through all datapoints and start getTotalDurationForState
		for _, state := range uniqueStateArray {
			go getTotalDurationForState(durationArray, stateArray, state, totalDurationChannel)
		}

		// get all results back
		for i := 0; i < len(uniqueStateArray); i++ {
			currentResult := <-totalDurationChannel
			if currentResult.err != nil {
				zap.S().Errorw(
					"Error in calculateDurations",
					"error", currentResult.err,
				)
				err = currentResult.err
				return
			}
			paretoEntry, ok := currentResult.returnValue.(datamodel.ParetoEntry)
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
	*/
	return

}
