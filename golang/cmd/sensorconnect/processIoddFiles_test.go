package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAddNewDeviceToIoddFilesAndMap(t *testing.T) {
	// first remove all files from specified path
	relativeDirectoryPath := "../sensorconnect/IoddFiles/"
	removeFilesFromDirectory(relativeDirectoryPath)

	//Declare Variables
	ioDeviceMap := make(map[IoddFilemapKey]IoDevice)
	var fileInfoSlice []os.FileInfo
	var ioddFilemapKey IoddFilemapKey
	ioddFilemapKey.DeviceId = 278531
	ioddFilemapKey.VendorId = 42
	var err error

	// execute function and check for errors
	ioDeviceMap, fileInfoSlice, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey, relativeDirectoryPath, ioDeviceMap, fileInfoSlice)
	if err != nil {
		t.Error(err)
	}
	// check if new entry exits for filemap Key
	if val, ok := ioDeviceMap[ioddFilemapKey]; !ok {
		fmt.Println(val)
		fmt.Println(ok)
		t.Error(err) // entry does not exist
	}
}

func TestRequestSaveIoddFile(t *testing.T) {
	var ioddFilemapKey IoddFilemapKey
	ioddFilemapKey.DeviceId = 278531
	ioddFilemapKey.VendorId = 42
	emptyIoDeviceMap := make(map[IoddFilemapKey]IoDevice)
	relativeDirectoryPath := "../sensorconnect/IoddFiles/"
	// first remove all files from specified path
	removeFilesFromDirectory(relativeDirectoryPath)
	err := RequestSaveIoddFile(ioddFilemapKey, emptyIoDeviceMap, relativeDirectoryPath)
	if err != nil {
		t.Error(err)
	}
	// Remove file after test again
	removeFilesFromDirectory(relativeDirectoryPath)
}

func TestReadIoddFiles(t *testing.T) {
	ioDeviceMap := make(map[IoddFilemapKey]IoDevice)
	var fileInfoSlice []os.FileInfo
	relativeDirectoryPath := "../sensorconnect/IoddFiles/"
	// first remove all files from specified path
	removeFilesFromDirectory(relativeDirectoryPath)
	var err error
	ioDeviceMap, fileInfoSlice, err = ReadIoddFiles(ioDeviceMap, fileInfoSlice, relativeDirectoryPath)
	// no changes in directory -> no new new files read
	if err != nil {
		t.Error(err)
	}

	var ioddFilemapKey IoddFilemapKey
	ioddFilemapKey.DeviceId = 278531
	ioddFilemapKey.VendorId = 42
	err = RequestSaveIoddFile(ioddFilemapKey, ioDeviceMap, relativeDirectoryPath)
	if err != nil {
		t.Error(err)
	}
	ioDeviceMap, _, err = ReadIoddFiles(ioDeviceMap, fileInfoSlice, relativeDirectoryPath)
	fmt.Println(ioDeviceMap)
	// check if new entry exits for filemap Key
	if val, ok := ioDeviceMap[ioddFilemapKey]; !ok {
		fmt.Println(val)
		fmt.Println(ok)
		t.Error(err) // entry does not exist
	}
	// Remove file after test again
	removeFilesFromDirectory(relativeDirectoryPath)
}

func removeFilesFromDirectory(relativeDirectoryPath string) {
	absoluteDirectoryPath, _ := filepath.Abs(relativeDirectoryPath)
	os.RemoveAll(absoluteDirectoryPath)
	os.MkdirAll(absoluteDirectoryPath, 0755)
}

func TestUnmarshalIoddFile_ifm(t *testing.T) {
	// first remove all files from specified path
	relativeDirectoryPath := "../sensorconnect/IoddFiles/"
	removeFilesFromDirectory(relativeDirectoryPath)

	//Declare Variables
	ioDeviceMap := make(map[IoddFilemapKey]IoDevice)
	var fileInfoSlice []os.FileInfo

	var ioddFilemapKey_IFM IoddFilemapKey
	ioddFilemapKey_IFM.DeviceId = 698
	ioddFilemapKey_IFM.VendorId = 310

	var ioddFilemapKey_rexroth IoddFilemapKey
	ioddFilemapKey_rexroth.DeviceId = 2228227
	ioddFilemapKey_rexroth.VendorId = 287

	var ioddFilemapKey_siemens IoddFilemapKey
	ioddFilemapKey_siemens.DeviceId = 278531
	ioddFilemapKey_siemens.VendorId = 42

	var ioddFilemapKey_IFMiodd IoddFilemapKey
	ioddFilemapKey_IFMiodd.DeviceId = 967
	ioddFilemapKey_IFMiodd.VendorId = 310
	var err error

	// execute function and check for errors
	ioDeviceMap, fileInfoSlice, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey_IFM, relativeDirectoryPath, ioDeviceMap, fileInfoSlice)
	if err != nil {
		t.Error(err)
	}
	// execute function and check for errors
	ioDeviceMap, fileInfoSlice, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey_rexroth, relativeDirectoryPath, ioDeviceMap, fileInfoSlice)
	if err != nil {
		t.Error(err)
	}

	// execute function and check for errors
	ioDeviceMap, fileInfoSlice, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey_siemens, relativeDirectoryPath, ioDeviceMap, fileInfoSlice)
	if err != nil {
		t.Error(err)
	}

	// execute function and check for errors
	ioDeviceMap, fileInfoSlice, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey_IFMiodd, relativeDirectoryPath, ioDeviceMap, fileInfoSlice)
	if err != nil {
		t.Error(err)
	}

	// Set io Device to ifm
	ioDevice := ioDeviceMap[ioddFilemapKey_IFM]

	//DeviceId: should give out 698
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceIdentity.DeviceId, 698) {
		t.Error()
	}
	//DeviceId: type should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceIdentity.DeviceId).Kind(), reflect.Int) {
		t.Error()
	}

	//VendorName: should give out "ifm electronic gmbh"
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceIdentity.VendorName, "ifm electronic gmbh") {
		t.Error()
	}
	//VendorName: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceIdentity.VendorName).Kind(), reflect.String) {
		t.Error()
	}

	//Check correct length of Text[] in ExternalTextCollection>PrimaryLanguage
	if !reflect.DeepEqual(len(ioDevice.ExternalTextCollection.PrimaryLanguage.Text), 177) {
		t.Error()
	}
	//Id: should give out "TI_ProductName0"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id, "TI_ProductName0") {
		t.Error()
	}
	//Id: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id).Kind(), reflect.String) {
		t.Error()
	}

	//Value: should give out "UGR500"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value, "UGR500") {
		t.Error()
	}
	//Value: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value).Kind(), reflect.String) {
		t.Error()
	}

	//bitLength (Datatype): should give out 32
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength, 32) {
		t.Error()
	}
	//bitLength (Datatype): should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength).Kind(), reflect.Int) {
		t.Error()
	}

	//BitLength (of SimpleDatatype): should be 4 here
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength, 4) {
		t.Error()
	}
	//BitLength (of SimpleDatatype): type should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength).Kind(), reflect.Int) {
		t.Error()
	}

	//xsi:type (of SimpleDatatype): should be UIntegerT
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.Type, "UIntegerT") {
		t.Error()
	}
	//xsi:type (of SimpleDatatype): should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.Type).Kind(), reflect.String) {
		t.Error()
	}

	//TextId (of RecordItem>Name): should be TI_PD_SV_2_Name
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].Name.TextId, "TI_PD_SV_2_Name") {
		t.Error()
	}
	//TextId (of RecordItem>Name): should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].Name.TextId).Kind(), reflect.String) {
		t.Error()
	}

	//BitOffset (of RecordItem): should be 4
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].BitOffset, 4) {
		t.Error()
	}
	//BitOffset (of RecordItem): should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].BitOffset).Kind(), reflect.Int) {
		t.Error()
	}

	//Check correct length of RecordItem[] in Datatype
	if !reflect.DeepEqual(len(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem), 4) {
		t.Error()
	}

	// Set io Device to rexroth
	ioDevice = ioDeviceMap[ioddFilemapKey_rexroth]

	//DeviceId: should give out 2228227
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceIdentity.DeviceId, 2228227) {
		t.Error()
	}
	//DeviceId: type should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceIdentity.DeviceId).Kind(), reflect.Int) {
		t.Error()
	}

	//VendorName: should give out "Bosch_Rexroth_AG"
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceIdentity.VendorName, "Bosch_Rexroth_AG") {
		t.Error()
	}
	//VendorName: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceIdentity.VendorName).Kind(), reflect.String) {
		t.Error()
	}

	//Check correct length of Text[] in ExternalTextCollection>PrimaryLanguage
	if !reflect.DeepEqual(len(ioDevice.ExternalTextCollection.PrimaryLanguage.Text), 98) {
		t.Error()
	}
	//Id: should give out "TI_VendorText"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id, "TI_VendorText") {
		t.Error()
	}
	//Id: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id).Kind(), reflect.String) {
		t.Error()
	}

	//Value: should give out "www.boschrexroth.com"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value, "www.boschrexroth.com") {
		t.Error()
	}
	//Value: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value).Kind(), reflect.String) {
		t.Error()
	}

	//bitLength (Datatype): should give out 16
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength, 16) {
		t.Error()
	}
	//bitLength (Datatype): should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength).Kind(), reflect.Int) {
		t.Error()
	}

	//BitLength (of SimpleDatatype): should be 0 here (zero because not given/specified in IODD file)
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength, 0) {
		t.Error()
	}
	//BitLength (of SimpleDatatype): type should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength).Kind(), reflect.Int) {
		t.Error()
	}

	//xsi:type (of SimpleDatatype): should be BooleanT
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.Type, "BooleanT") {
		t.Error()
	}
	//xsi:type (of SimpleDatatype): should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.Type).Kind(), reflect.String) {
		t.Error()
	}

	//TextId (of RecordItem>Name): should be DT_RI_Name3640Errorbit
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].Name.TextId, "DT_RI_Name3640Errorbit") {
		t.Error()
	}
	//TextId (of RecordItem>Name): should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].Name.TextId).Kind(), reflect.String) {
		t.Error()
	}

	//BitOffset (of RecordItem): should be 1
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].BitOffset, 1) {
		t.Error()
	}
	//BitOffset (of RecordItem): should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].BitOffset).Kind(), reflect.Int) {
		t.Error()
	}

	//Check correct length of RecordItem[] in Datatype
	if !reflect.DeepEqual(len(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem), 3) {
		t.Error()
	}

	// Set io Device to siemens
	ioDevice = ioDeviceMap[ioddFilemapKey_siemens]

	//DeviceId: should give out 278531
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceIdentity.DeviceId, 278531) {
		t.Error()
	}
	//DeviceId: type should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceIdentity.DeviceId).Kind(), reflect.Int) {
		t.Error()
	}

	//VendorName: should give out "Siemens AG"
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceIdentity.VendorName, "Siemens AG") {
		t.Error()
	}
	//VendorName: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceIdentity.VendorName).Kind(), reflect.String) {
		t.Error()
	}

	//Check correct length of Text[] in ExternalTextCollection>PrimaryLanguage
	if !reflect.DeepEqual(len(ioDevice.ExternalTextCollection.PrimaryLanguage.Text), 123) {
		fmt.Println(len(ioDevice.ExternalTextCollection.PrimaryLanguage.Text))
		t.Error()
	}
	//Id: should give out "TI_VendorText"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id, "TI_VendorText") {
		t.Error()
	}
	//Id: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id).Kind(), reflect.String) {
		t.Error()
	}

	//Value: should give out "Siemens AG"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value, "Siemens AG") {
		t.Error()
	}
	//Value: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value).Kind(), reflect.String) {
		t.Error()
	}

	//bitLength (Datatype): should give out 16
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength, 16) {
		t.Error()
	}
	//bitLength (Datatype): should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength).Kind(), reflect.Int) {
		t.Error()
	}

	//BitLength (of SimpleDatatype): should be 0 here (zero because not given/specified in IODD file)
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength, 0) {
		t.Error()
	}
	//BitLength (of SimpleDatatype): type should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength).Kind(), reflect.Int) {
		t.Error()
	}

	//xsi:type (of SimpleDatatype): should be "" (because of different structure: without SimpleDatatype)
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.Type, "") {
		t.Error()
	}
	//xsi:type (of SimpleDatatype): should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.Type).Kind(), reflect.String) {
		t.Error()
	}

	//TextId (of RecordItem>Name): should be TI_PaeError
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].Name.TextId, "TI_PaeError") {
		t.Error()
	}
	//TextId (of RecordItem>Name): should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].Name.TextId).Kind(), reflect.String) {
		t.Error()
	}

	//BitOffset (of RecordItem): should be 9
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].BitOffset, 9) {
		t.Error()
	}
	//BitOffset (of RecordItem): should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].BitOffset).Kind(), reflect.Int) {
		t.Error()
	}

	//Check correct length of RecordItem[] in Datatype
	if !reflect.DeepEqual(len(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem), 16) {
		t.Error()
	}

	// Set io Device to rexroth
	ioDevice = ioDeviceMap[ioddFilemapKey_IFMiodd]

	//DeviceId: should give out 967
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceIdentity.DeviceId, 967) {
		t.Error()
	}
	//DeviceId: type should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceIdentity.DeviceId).Kind(), reflect.Int) {
		t.Error()
	}

	//VendorName: should give out "ifm electronic gmbh"
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceIdentity.VendorName, "ifm electronic gmbh") {
		t.Error()
	}
	//VendorName: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceIdentity.VendorName).Kind(), reflect.String) {
		t.Error()
	}

	//Check correct length of Text[] in ExternalTextCollection>PrimaryLanguage
	if !reflect.DeepEqual(len(ioDevice.ExternalTextCollection.PrimaryLanguage.Text), 144) {
		t.Error()
	}
	//Id: should give out "TI_ProductName0"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id, "TI_ProductName0") {
		t.Error()
	}
	//Id: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id).Kind(), reflect.String) {
		t.Error()
	}

	//Value: should give out "DTI410"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value, "DTI410") {
		t.Error()
	}
	//Value: type should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value).Kind(), reflect.String) {
		t.Error()
	}

	//bitLength (Datatype): should give out 256
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength, 256) {
		t.Error()
	}
	//bitLength (Datatype): should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength).Kind(), reflect.Int) {
		t.Error()
	}

	//BitLength (of SimpleDatatype): should be 0 here (because not specified)
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength, 0) {
		t.Error()
	}
	//BitLength (of SimpleDatatype): type should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength).Kind(), reflect.Int) {
		t.Error()
	}

	//xsi:type (of SimpleDatatype): should be BooleanT
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.Type, "BooleanT") {
		t.Error()
	}
	//xsi:type (of SimpleDatatype): should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.Type).Kind(), reflect.String) {
		t.Error()
	}

	//TextId (of RecordItem>Name): should be TI_PD_SV_IN_2_Name
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].Name.TextId, "TI_PD_SV_IN_2_Name") {
		t.Error()
	}
	//TextId (of RecordItem>Name): should be string
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].Name.TextId).Kind(), reflect.String) {
		t.Error()
	}

	//BitOffset (of RecordItem): should be 243
	if !reflect.DeepEqual(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].BitOffset, 243) {
		t.Error()
	}
	//BitOffset (of RecordItem): should be int
	if !reflect.DeepEqual(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].BitOffset).Kind(), reflect.Int) {
		t.Error()
	}

	//Check correct length of RecordItem[] in Datatype
	if !reflect.DeepEqual(len(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem), 8) {
		t.Error()
	}
}
