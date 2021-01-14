package main

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
)

/*
loggingTimestamp := time.Now()

	if parentSpan != nil && logData {
		logObject("addUnknownMicrostops", "stateArray", loggingTimestamp, stateArray)
		logObject("addUnknownMicrostops", "configuration", loggingTimestamp, configuration)
	}
*/

func TestAddLowSpeedStates_1(t *testing.T) {
	var stateArray []datamodel.StateEntry
	var configuration datamodel.CustomerConfiguration
	var countSlice []datamodel.CountEntry
	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_stateArray_1601391491.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_configuration_1601391491.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_countSlice_1601391491.golden", &countSlice)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_processedStateArray_1601391491.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	stateArray = ConvertOldToNewStateEntryArray(stateArray)

	processedStateArrayFresh, err := addLowSpeedStates(nil, 0, stateArray, countSlice, configuration)
	if err != nil {
		t.Error()
	}

	processedStateArrayFresh = ConvertNewToOldStateEntryArray(processedStateArrayFresh)

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}

}

func TestAddLowSpeedStates_2(t *testing.T) { // Complex
	var stateArray []datamodel.StateEntry
	var configuration datamodel.CustomerConfiguration
	var countSlice []datamodel.CountEntry
	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_stateArray_1601392511.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_configuration_1601392511.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_countSlice_1601392511.golden", &countSlice)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_processedStateArray_1601392511.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	stateArray = ConvertOldToNewStateEntryArray(stateArray)

	processedStateArrayFresh, err := addLowSpeedStates(nil, 0, stateArray, countSlice, configuration)
	if err != nil {
		t.Error()
	}

	processedStateArrayFresh = ConvertNewToOldStateEntryArray(processedStateArrayFresh)

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}

}

func TestAddLowSpeedStates_NoLowSpeed_1(t *testing.T) {
	var stateArray []datamodel.StateEntry
	var configuration datamodel.CustomerConfiguration
	var countSlice []datamodel.CountEntry
	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_stateArray_1601392325.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_configuration_1601392325.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_countSlice_1601392325.golden", &countSlice)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addLowSpeedStates_processedStateArray_1601392325.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	stateArray = ConvertOldToNewStateEntryArray(stateArray)

	processedStateArrayFresh, err := addLowSpeedStates(nil, 0, stateArray, countSlice, configuration)
	if err != nil {
		t.Error()
	}

	processedStateArrayFresh = ConvertNewToOldStateEntryArray(processedStateArrayFresh)

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}

}

func TestAddUnknownMicrostops_OnlyMicrostops(t *testing.T) {
	var stateArray []datamodel.StateEntry
	var configuration datamodel.CustomerConfiguration
	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/addUnknownMicrostops_stateArray_1601323210.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addUnknownMicrostops_configuration_1601323210.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addUnknownMicrostops_processedStateArray_1601323210.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	stateArray = ConvertOldToNewStateEntryArray(stateArray)

	processedStateArrayFresh, err := addUnknownMicrostops(nil, stateArray, configuration)
	if err != nil {
		t.Error()
	}

	processedStateArrayFresh = ConvertNewToOldStateEntryArray(processedStateArrayFresh)

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}

}

func TestAddUnknownMicrostops_OnlyMicrostopsOneStop(t *testing.T) {
	var stateArray []datamodel.StateEntry
	var configuration datamodel.CustomerConfiguration
	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/addUnknownMicrostops_stateArray_1601323241.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addUnknownMicrostops_configuration_1601323241.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addUnknownMicrostops_processedStateArray_1601323241.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	stateArray = ConvertOldToNewStateEntryArray(stateArray)

	processedStateArrayFresh, err := addUnknownMicrostops(nil, stateArray, configuration)
	if err != nil {
		t.Error()
	}

	processedStateArrayFresh = ConvertNewToOldStateEntryArray(processedStateArrayFresh)

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}
}

func TestAddUnknownMicrostops_Complex(t *testing.T) {
	var stateArray []datamodel.StateEntry
	var configuration datamodel.CustomerConfiguration
	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/addUnknownMicrostops_stateArray_1601323170.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addUnknownMicrostops_configuration_1601323170.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/addUnknownMicrostops_processedStateArray_1601323170.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	stateArray = ConvertOldToNewStateEntryArray(stateArray)

	processedStateArrayFresh, err := addUnknownMicrostops(nil, stateArray, configuration)
	if err != nil {
		t.Error()
	}

	processedStateArrayFresh = ConvertNewToOldStateEntryArray(processedStateArrayFresh)

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}
}

func TestIsPerformanceLoss_1(t *testing.T) {

	var configuration datamodel.CustomerConfiguration

	configuration.AvailabilityLossStates = append(configuration.AvailabilityLossStates, 40000, 180000, 190000, 200000)
	configuration.PerformanceLossStates = append(configuration.PerformanceLossStates, 20000, 40100, 70000)

	var state int32 = 40100

	result := IsPerformanceLoss(state, configuration)

	if !reflect.DeepEqual(result, true) {
		t.Error()
	}
}

func TestIsPerformanceLoss_2(t *testing.T) {

	var configuration datamodel.CustomerConfiguration

	configuration.AvailabilityLossStates = append(configuration.AvailabilityLossStates, 40000, 180000, 190000, 200000)
	configuration.PerformanceLossStates = append(configuration.PerformanceLossStates, 20000, 40100, 70000)

	var state int32 = 40200

	result := IsPerformanceLoss(state, configuration)

	if !reflect.DeepEqual(result, false) {
		t.Error()
	}
}

func TestIsAvailabilityLoss_1(t *testing.T) {

	var configuration datamodel.CustomerConfiguration

	configuration.AvailabilityLossStates = append(configuration.AvailabilityLossStates, 40000, 180000, 190000, 200000)
	configuration.PerformanceLossStates = append(configuration.PerformanceLossStates, 20000, 40100, 70000)

	var state int32 = 40100

	result := IsAvailabilityLoss(state, configuration)

	if !reflect.DeepEqual(result, false) {
		t.Error()
	}
}

func TestIsAvailabilityLoss_2(t *testing.T) {

	var configuration datamodel.CustomerConfiguration

	configuration.AvailabilityLossStates = append(configuration.AvailabilityLossStates, 40000, 180000, 190000, 200000)
	configuration.PerformanceLossStates = append(configuration.PerformanceLossStates, 20000, 40100, 70000)

	var state int32 = 40200

	result := IsAvailabilityLoss(state, configuration)

	if !reflect.DeepEqual(result, true) {
		t.Error()
	}
}

func TestAutomaticallyIdentifyChangeovers_Disabled_1(t *testing.T) {

	var stateArray []datamodel.StateEntry
	var orderArray []datamodel.OrdersRaw
	var configuration datamodel.CustomerConfiguration
	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_stateArray_1610629712.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_orderArray_1610629712.golden", &orderArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_configuration_1610629712.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_processedStateArray_1610629712.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	processedStateArrayFresh, err := automaticallyIdentifyChangeovers(nil, stateArray, orderArray, configuration)
	if err != nil {
		t.Error()
	}

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}
}

func TestAutomaticallyIdentifyChangeovers_Enabled_1(t *testing.T) {

	var stateArray []datamodel.StateEntry
	var orderArray []datamodel.OrdersRaw
	var configuration datamodel.CustomerConfiguration
	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_stateArray_1610629712_v2.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_orderArray_1610629712_v2.golden", &orderArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_configuration_1610629712_v2.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_processedStateArray_1610629712_v2.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	processedStateArrayFresh, err := automaticallyIdentifyChangeovers(nil, stateArray, orderArray, configuration)
	if err != nil {
		t.Error()
	}

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}
}

func TestAutomaticallyIdentifyChangeovers_Enabled_2(t *testing.T) {

	var stateArray []datamodel.StateEntry
	var orderArray []datamodel.OrdersRaw
	var configuration datamodel.CustomerConfiguration
	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_stateArray_1610630858.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_orderArray_1610630858.golden", &orderArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_configuration_1610630858.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_processedStateArray_1610630858.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	processedStateArrayFresh, err := automaticallyIdentifyChangeovers(nil, stateArray, orderArray, configuration)
	if err != nil {
		t.Error()
	}

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}
}

/*
func TestProcessStates_Complex_1(t *testing.T) {
	var stateArray []datamodel.StateEntry
	var rawShifts []ShiftEntry
	var countSlice []datamodel.CountEntry
	var configuration datamodel.CustomerConfiguration
	var from time.Time
	var to time.Time

	var processedStateArray []datamodel.StateEntry

	err := internal.Load("../../test/factoryinsight/testfiles/processStates_stateArray_1601585625.golden", &stateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/processStates_rawShifts_1601585625.golden", &rawShifts)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/processStates_countSlice_1601585625.golden", &countSlice)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/processStates_configuration_1601585625.golden", &configuration)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/processStates_from_1601585625.golden", &from)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	err = internal.Load("../../test/factoryinsight/testfiles/processStates_to_1601585625.golden", &to)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	err = internal.Load("../../test/factoryinsight/testfiles/processStates_processedStateArray_1601585625.golden", &processedStateArray)
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	processedStateArrayFresh, err := processStates(nil, 0, stateArray, rawShifts, countSlice, from, to, configuration)
	if err != nil {
		t.Error()
	}

	if !reflect.DeepEqual(processedStateArrayFresh, processedStateArray) {
		fmt.Println(processedStateArrayFresh)
		fmt.Println(processedStateArray)
		t.Error()
	}
}
*/
