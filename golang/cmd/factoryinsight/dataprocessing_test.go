package main

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"

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

	from, err := time.Parse(time.RFC3339, "2021-01-09T07:48:21.616Z")
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	to, err := time.Parse(time.RFC3339, "2021-01-09T11:59:37.107Z")
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_stateArray_1610629712.golden", &stateArray)
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

	processedStateArrayFresh, err := automaticallyIdentifyChangeovers(nil, stateArray, orderArray, from, to, configuration)
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

	from, err := time.Parse(time.RFC3339, "2021-01-09T07:48:21.616Z")
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	to, err := time.Parse(time.RFC3339, "2021-01-09T11:59:37.107Z")
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_stateArray_1610629712_v2.golden", &stateArray)
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

	processedStateArrayFresh, err := automaticallyIdentifyChangeovers(nil, stateArray, orderArray, from, to, configuration)
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

	from, err := time.Parse(time.RFC3339, "2021-01-09T09:26:11.26Z")
	if err != nil {
		fmt.Println(err)
		t.Error()
	}
	to, err := time.Parse(time.RFC3339, "2021-01-09T10:30:15.861Z")
	if err != nil {
		fmt.Println(err)
		t.Error()
	}

	err = internal.Load("../../test/factoryinsight/testfiles/AutomaticallyIdentifyChangeovers_stateArray_1610630858.golden", &stateArray)
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

	processedStateArrayFresh, err := automaticallyIdentifyChangeovers(nil, stateArray, orderArray, from, to, configuration)
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

func Test_processStatesOptimized(t *testing.T) {
	type args struct {
		assetID       uint32
		stateArray    []datamodel.StateEntry
		rawShifts     []datamodel.ShiftEntry
		countSlice    []datamodel.CountEntry
		orderArray    []datamodel.OrdersRaw
		from          time.Time
		to            time.Time
		configuration datamodel.CustomerConfiguration
	}
	tests := []struct {
		name                    string
		args                    args
		wantProcessedStateArray []datamodel.StateEntry
		wantErr                 bool
		goldenTimestamp         string
	}{
		{
			name:            "shifts1",
			goldenTimestamp: "1614606163",
		},
		{
			name:            "noShift before unknown stop",
			goldenTimestamp: "1614607503",
		},
		{
			name:            "multiple noShifts during one long stop #145",
			goldenTimestamp: "1614607583",
		},
	}
	for _, tt := range tests {
		// loading golden files
		err := internal.Load("../../test/factoryinsight/testfiles/"+"processStatesOptimized"+"_processedStateArray_"+tt.goldenTimestamp+".golden", &tt.wantProcessedStateArray)
		if err != nil {
			fmt.Println(err)
			t.Error()
		}

		err = internal.Load("../../test/factoryinsight/testfiles/"+"processStatesOptimized"+"_stateArray_"+tt.goldenTimestamp+".golden", &tt.args.stateArray)
		if err != nil {
			fmt.Println(err)
			t.Error()
		}

		err = internal.Load("../../test/factoryinsight/testfiles/"+"processStatesOptimized"+"_rawShifts_"+tt.goldenTimestamp+".golden", &tt.args.rawShifts)
		if err != nil {
			fmt.Println(err)
			t.Error()
		}

		err = internal.Load("../../test/factoryinsight/testfiles/"+"processStatesOptimized"+"_countSlice_"+tt.goldenTimestamp+".golden", &tt.args.countSlice)
		if err != nil {
			fmt.Println(err)
			t.Error()
		}

		err = internal.Load("../../test/factoryinsight/testfiles/"+"processStatesOptimized"+"_orderArray_"+tt.goldenTimestamp+".golden", &tt.args.orderArray)
		if err != nil {
			fmt.Println(err)
			t.Error()
		}

		err = internal.Load("../../test/factoryinsight/testfiles/"+"processStatesOptimized"+"_from_"+tt.goldenTimestamp+".golden", &tt.args.from)
		if err != nil {
			fmt.Println(err)
			t.Error()
		}

		err = internal.Load("../../test/factoryinsight/testfiles/"+"processStatesOptimized"+"_to_"+tt.goldenTimestamp+".golden", &tt.args.to)
		if err != nil {
			fmt.Println(err)
			t.Error()
		}

		err = internal.Load("../../test/factoryinsight/testfiles/"+"processStatesOptimized"+"_configuration_"+tt.goldenTimestamp+".golden", &tt.args.configuration)
		if err != nil {
			fmt.Println(err)
			t.Error()
		}

		// Executing tests
		t.Run(tt.name, func(t *testing.T) {
			gotProcessedStateArray, err := processStatesOptimized(nil, tt.args.assetID, tt.args.stateArray, tt.args.rawShifts, tt.args.countSlice, tt.args.orderArray, tt.args.from, tt.args.to, tt.args.configuration)
			if (err != nil) != tt.wantErr {
				t.Errorf("processStatesOptimized() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotProcessedStateArray, tt.wantProcessedStateArray) {
				t.Errorf("processStatesOptimized() got / want")
				t.Errorf("%v", gotProcessedStateArray)
				t.Errorf("%v", tt.wantProcessedStateArray)
			}
		})
	}
}

func TestSliceContainsInt(t *testing.T) {

	var slice [][]interface{}

	var row1 []interface{}
	var row2 []interface{}
	var row3 []interface{}
	row1 = append(row1, 1)
	row1 = append(row1, "test")
	row1 = append(row1, 45)
	row2 = append(row2, 2)
	row2 = append(row2, "testas")
	row2 = append(row2, 455)
	row3 = append(row3, 3)
	row3 = append(row3, "testaser")
	row3 = append(row3, 545)
	slice = append(slice, row1)
	slice = append(slice, row2)
	slice = append(slice, row3)

	resultContains, resultIndex := SliceContainsInt(slice, 2, 0)

	if !reflect.DeepEqual(resultContains, true) {
		t.Error()
	}
	if !reflect.DeepEqual(resultIndex, 1) {
		t.Error()
	}

	resultContains, resultIndex = SliceContainsInt(slice, 4, 0)
	if !reflect.DeepEqual(resultContains, false) {
		t.Error()
	}
	if !reflect.DeepEqual(resultIndex, 0) {
		t.Error()
	}

}

func TestChangeOutputFormat(t *testing.T) {

	var data [][]interface{}
	var dataExtended [][]interface{}
	var columnNames []string

	columnNames = []string{"uid", "aid", "Force"}
	var row1 []interface{}
	var row2 []interface{}
	var row3 []interface{}
	row1 = append(row1, 1)
	row1 = append(row1, "A102")
	row1 = append(row1, 45.5)
	row2 = append(row2, 2)
	row2 = append(row2, "A103")
	row2 = append(row2, 455)
	row3 = append(row3, 3)
	row3 = append(row3, "A104")
	row3 = append(row3, 545)
	data = append(data, row1)
	data = append(data, row2)
	data = append(data, row3)

	//Handling if inputColumnName is not new
	dataOutput, columnNamesOutput, columnIndex := ChangeOutputFormat(data, columnNames, "Force")
	if !reflect.DeepEqual(dataOutput, data) {
		t.Error()
	}
	if !reflect.DeepEqual(columnNamesOutput, columnNames) {
		t.Error()
	}
	if !reflect.DeepEqual(columnIndex, 2) {
		t.Error()
	}

	//Handling if inputColumnName is new
	columnNamesExtended := append(columnNames, "Temperature")
	row1 = append(row1, nil)
	row2 = append(row2, nil)
	row3 = append(row3, nil)
	dataExtended = append(dataExtended, row1)
	dataExtended = append(dataExtended, row2)
	dataExtended = append(dataExtended, row3)

	dataOutput, columnNamesOutput, columnIndex = ChangeOutputFormat(data, columnNames, "Temperature")
	fmt.Printf("%s\n", columnNamesOutput)
	fmt.Printf("%s\n", dataOutput)
	if !reflect.DeepEqual(dataOutput, dataExtended) {
		t.Error()
	}
	if !reflect.DeepEqual(columnNamesOutput, columnNamesExtended) {
		t.Error()
	}
	if !reflect.DeepEqual(columnIndex, 3) {
		t.Error()
	}
}

func TestLengthenSliceToFitNames(t *testing.T) {

	var sliceTooSmall []interface{}
	var sliceTooSmallExpectedOutput []interface{}
	var sliceExact []interface{}
	var sliceTooLong []interface{}
	var columnNames []string
	columnNames = []string{"uid", "aid", "Force"}
	sliceTooSmall = append(sliceTooSmall, 1)
	sliceTooSmall = append(sliceTooSmall, "A102")
	sliceTooSmallExpectedOutput = append(sliceTooSmallExpectedOutput, 1)
	sliceTooSmallExpectedOutput = append(sliceTooSmallExpectedOutput, "A102")
	sliceTooSmallExpectedOutput = append(sliceTooSmallExpectedOutput, nil)
	sliceExact = append(sliceExact, 2)
	sliceExact = append(sliceExact, "A103")
	sliceExact = append(sliceExact, 455)
	sliceTooLong = append(sliceTooLong, 3)
	sliceTooLong = append(sliceTooLong, "A104")
	sliceTooLong = append(sliceTooLong, 545)
	sliceTooLong = append(sliceTooLong, "EntryTooMuch")

	//Handling if input slice too small
	sliceOutput := LengthenSliceToFitNames(sliceTooSmall, columnNames)
	if !reflect.DeepEqual(sliceOutput, sliceTooSmallExpectedOutput) {
		t.Error()
	}
	//Handling if input slice correct
	sliceOutput = LengthenSliceToFitNames(sliceExact, columnNames)
	if !reflect.DeepEqual(sliceOutput, sliceExact) {
		t.Error()
	}
	//Handling if input slice too large
	sliceOutput = LengthenSliceToFitNames(sliceTooLong, columnNames)
	fmt.Printf("%s\n", sliceOutput)
	if !reflect.DeepEqual(sliceOutput, sliceOutput) {
		t.Error()
	}
}

func TestCreateNewRowInData(t *testing.T) {

	var UID int
	var AID string
	var timestampBegin time.Time
	var timestampEnd sql.NullTime
	var productID int
	var isScrap bool
	var valueName sql.NullString
	var value sql.NullFloat64
	var data [][]interface{}
	var columnNames []string

	UID = 12
	AID = "A106"
	timestampBegin = time.Unix(32023904, 0)
	timestampEnd.Time = time.Unix(32023999, 0)
	timestampEnd.Valid = true
	productID = 23438
	isScrap = false
	valueName.String = "Force"
	valueName.Valid = true
	value.Float64 = 1.38
	value.Valid = true

	columnNames = []string{"UID", "AID", "TimestampBegin", "TimestampEnd", "ProductID", "IsScrap", "Torque", "Speed", "Force"}
	var row1 []interface{}
	var row2 []interface{}

	row1TimeBegin := time.Unix(32023904, 0)
	row1TimeEnd := time.Unix(32023898, 0)
	row2TimeBegin := time.Unix(32024904, 0)
	row2TimeEnd := time.Unix(32024898, 0)

	row1 = append(row1, 1)
	row1 = append(row1, "A102")
	row1 = append(row1, float64(row1TimeBegin.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	row1 = append(row1, float64(row1TimeEnd.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	row1 = append(row1, 10011)
	row1 = append(row1, false)
	row1 = append(row1, 45.6)
	row1 = append(row1, 1.13)
	row1 = append(row1, 0.023)

	row2 = append(row2, 2)
	row2 = append(row2, "A103")
	row2 = append(row2, float64(row2TimeBegin.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	row2 = append(row2, float64(row2TimeEnd.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	row2 = append(row2, 10011)
	row2 = append(row2, false)
	row2 = append(row2, 44.6)
	row2 = append(row2, 1.01)
	row2 = append(row2, 0.021)

	data = append(data, row1)
	data = append(data, row2)

	dataOutput := CreateNewRowInData(data, columnNames, 7, UID, AID,
		timestampBegin, timestampEnd, productID, isScrap, valueName, value)
	fmt.Printf("%s\n", dataOutput)
	if !reflect.DeepEqual(len(dataOutput), 3) {
		t.Error()
	}

	if !reflect.DeepEqual(dataOutput[2][6], nil) {
		t.Error()
	}
	if !reflect.DeepEqual(dataOutput[2][7], value.Float64) {
		t.Error()
	}
	if !reflect.DeepEqual(dataOutput[2][8], nil) {
		t.Error()
	}
	if !reflect.DeepEqual(len(dataOutput[2]), 9) {
		t.Error()
	}
}

func TestCheckOutputDimensions(t *testing.T) {
	var data [][]interface{}
	var columnNames []string

	columnNames = []string{"UID", "AID", "TimestampBegin", "TimestampEnd", "ProductID", "IsScrap", "Torque", "Speed", "Force"}
	var row1 []interface{}
	var row2 []interface{}
	var rowTooShort []interface{}
	var rowTooLong []interface{}

	row1TimeBegin := time.Unix(32023904, 0)
	row1TimeEnd := time.Unix(32023898, 0)
	row2TimeBegin := time.Unix(32024904, 0)
	row2TimeEnd := time.Unix(32024898, 0)

	row1 = append(row1, 1)
	row1 = append(row1, "A102")
	row1 = append(row1, float64(row1TimeBegin.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	row1 = append(row1, float64(row1TimeEnd.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	row1 = append(row1, 10011)
	row1 = append(row1, false)
	row1 = append(row1, 45.6)
	row1 = append(row1, 1.13)
	row1 = append(row1, 0.023)

	row2 = append(row2, 2)
	row2 = append(row2, "A103")
	row2 = append(row2, float64(row2TimeBegin.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	row2 = append(row2, float64(row2TimeEnd.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	row2 = append(row2, 10011)
	row2 = append(row2, false)
	row2 = append(row2, 44.6)
	row2 = append(row2, 1.01)
	row2 = append(row2, 0.021)

	rowTooShort = append(rowTooShort, 2)
	rowTooShort = append(rowTooShort, "A103")
	rowTooShort = append(rowTooShort, float64(row2TimeBegin.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	rowTooShort = append(rowTooShort, float64(row2TimeEnd.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	rowTooShort = append(rowTooShort, 10011)
	rowTooShort = append(rowTooShort, false)
	rowTooShort = append(rowTooShort, 44.6)
	rowTooShort = append(rowTooShort, 1.01)

	rowTooLong = append(rowTooLong, 2)
	rowTooLong = append(rowTooLong, "A103")
	rowTooLong = append(rowTooLong, float64(row2TimeBegin.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	rowTooLong = append(rowTooLong, float64(row2TimeEnd.UnixNano()/(int64(time.Millisecond)/int64(time.Nanosecond))))
	rowTooLong = append(rowTooLong, 10011)
	rowTooLong = append(rowTooLong, false)
	rowTooLong = append(rowTooLong, 44.6)
	rowTooLong = append(rowTooLong, 1.01)
	rowTooLong = append(rowTooLong, 0.021)
	rowTooLong = append(rowTooLong, "entryTooMuch")

	data = append(data, row1)
	data = append(data, row2)

	fmt.Printf("%s\n", data)

	//Case: No Error
	err := CheckOutputDimensions(data, columnNames)
	if !reflect.DeepEqual(err, nil) {
		t.Error()
	}

	//Case: Error because last row too short
	data = append(data, rowTooShort)
	err = nil
	err = CheckOutputDimensions(data, columnNames)
	if err == nil {
		t.Error()
	}

	//Case: Error because last row too long
	data[2] = rowTooLong
	fmt.Printf("%s\n", data)
	err = nil
	err = CheckOutputDimensions(data, columnNames)
	if err == nil {
		t.Error()
	}
}
