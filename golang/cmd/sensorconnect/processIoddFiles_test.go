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

package main

import (
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const relativeIODDFilePath = "../sensorconnect/IoddFiles/"

func TestAddNewDeviceToIoddFilesAndMap(t *testing.T) {
	internal.InitCacheWithoutRedis()
	// first remove all files from specified path
	err := removeFilesFromDirectory()
	if err != nil {
		t.Errorf("removeFilesFromDirectory failed: %v", err)
	}

	// Declare Variables
	var fileInfoSlice []fs.DirEntry
	var ioddFilemapKey IoddFilemapKey
	ioddFilemapKey.DeviceId = 278531
	ioddFilemapKey.VendorId = 42

	// execute function and check for errors
	fileInfoSlice, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey, relativeIODDFilePath, fileInfoSlice, false)
	if err != nil {
		t.Error(err)
	}
	// check if new entry exits for filemap Key
	if _, ok := ioDeviceMap.Load(ioddFilemapKey); !ok {
		fmt.Println(ok)
		t.Error(err) // entry does not exist
	}
	// check if fileInfoSlice is correctly changed
	name := getNamesOfFileInfo(fileInfoSlice)
	if !reflect.DeepEqual(name[0], "Siemens-SIRIUS-3SU1-4DI4DQ-20160602-IODD1.1.xml") {
		t.Error(name[0])
	}
	if len(name) != 1 {
		t.Error()
	}
}

func TestRequestSaveIoddFile(t *testing.T) {
	internal.InitCacheWithoutRedis()
	var ioddFilemapKey IoddFilemapKey
	ioddFilemapKey.DeviceId = 278531
	ioddFilemapKey.VendorId = 42
	// first remove all files from specified path
	err := removeFilesFromDirectory()
	if err != nil {
		zap.S().Errorf("removeFilesFromDirectory failed: %v", err)
	}
	ioDeviceMap.Delete(ioddFilemapKey)
	err = RequestSaveIoddFile(ioddFilemapKey, relativeIODDFilePath, false)
	if err != nil {
		t.Error(err)
	}
	// Remove file after test again
	err = removeFilesFromDirectory()
	if err != nil {
		t.Errorf("removeFilesFromDirectory failed: %v", err)
	}
}

func TestReadIoddFiles(t *testing.T) {
	internal.InitCacheWithoutRedis()
	var fileInfoSlice []fs.DirEntry
	// first remove all files from specified path
	err := removeFilesFromDirectory()
	if err != nil {
		t.Errorf("removeFilesFromDirectory failed: %v", err)
	}
	fileInfoSlice, err = ReadIoddFiles(fileInfoSlice, relativeIODDFilePath)
	// no changes in directory -> no new new files read
	if err != nil {
		t.Error(err)
	}

	var ioddFilemapKey IoddFilemapKey
	ioddFilemapKey.DeviceId = 278531
	ioddFilemapKey.VendorId = 42
	ioDeviceMap.Delete(ioddFilemapKey)
	err = RequestSaveIoddFile(ioddFilemapKey, relativeIODDFilePath, false)
	if err != nil {
		t.Error(err)
	}
	_, err = ReadIoddFiles(fileInfoSlice, relativeIODDFilePath)
	// check if new entry exits for filemap Key
	if _, ok := ioDeviceMap.Load(ioddFilemapKey); !ok {
		fmt.Println(ok)
		t.Error(err) // entry does not exist
	}
	// Remove file after test again
	err = removeFilesFromDirectory()
	if err != nil {
		t.Errorf("removeFilesFromDirectory failed: %v", err)
	}
}

// Deletes complete directory and creates new one
func removeFilesFromDirectory() error {
	absoluteDirectoryPath, err := filepath.Abs(relativeIODDFilePath)
	if err != nil {
		return err
	}
	err = os.RemoveAll(absoluteDirectoryPath)
	if err != nil {
		return err
	}
	err = os.MkdirAll(absoluteDirectoryPath, 0755)
	if err != nil {
		return err
	}
	return nil
}

func TestUnmarshalIoddFiles(t *testing.T) {
	internal.InitCacheWithoutRedis()
	// first remove all files from specified path
	err := removeFilesFromDirectory()
	if err != nil {
		t.Errorf("removeFilesFromDirectory failed: %v", err)
	}
	// Declare Variables
	var fileInfoSlice []fs.DirEntry

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

	// execute function and check for errors
	_, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey_IFM, relativeIODDFilePath, fileInfoSlice, false)
	if err != nil {
		t.Error(err)
	}
	// execute function and check for errors
	_, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey_rexroth, relativeIODDFilePath, fileInfoSlice, false)
	if err != nil {
		t.Error(err)
	}

	// execute function and check for errors
	_, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey_siemens, relativeIODDFilePath, fileInfoSlice, false)
	if err != nil {
		t.Error(err)
	}

	// execute function and check for errors
	_, err = AddNewDeviceToIoddFilesAndMap(ioddFilemapKey_IFMiodd, relativeIODDFilePath, fileInfoSlice, false)
	if err != nil {
		t.Error(err)
	}

	// Set io Device to ifm
	ioDeviceInterface, ok := ioDeviceMap.Load(ioddFilemapKey_IFM)
	if !ok {
		t.Error("ioDeviceInterface not found")
	}
	var ioDevice IoDevice
	ioDevice, ok = ioDeviceInterface.(IoDevice)
	if !ok {
		t.Error("ioDevice is not of type IoDevice")
	}
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

	//Id: should give out "TI_ProductName0"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id, "TI_ProductName0") {
		t.Error()
	}
	//Id: type should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id).Kind(),
		reflect.String) {
		t.Error()
	}

	//Value: should give out "UGR500"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value, "UGR500") {
		t.Error()
	}
	//Value: type should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value).Kind(),
		reflect.String) {
		t.Error()
	}

	// bitLength (Datatype): should give out 32
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength,
		uint(32)) {
		t.Error()
	}
	// bitLength (Datatype): should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength).Kind(),
		reflect.Uint) {
		t.Error()
	}

	// BitLength (of SimpleDatatype): should be 4 here
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength,
		uint(4)) {
		t.Error()
	}
	// BitLength (of SimpleDatatype): type should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength).Kind(),
		reflect.Uint) {
		t.Error()
	}

	//xsi:type (of SimpleDatatype): should be UIntegerT
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.Type,
		"UIntegerT") {
		t.Error()
	}
	//xsi:type (of SimpleDatatype): should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.Type).Kind(),
		reflect.String) {
		t.Error()
	}

	// TextId (of RecordItem>Name): should be TI_PD_SV_2_Name
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].Name.TextId,
		"TI_PD_SV_2_Name") {
		t.Error()
	}
	// TextId (of RecordItem>Name): should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].Name.TextId).Kind(),
		reflect.String) {
		t.Error()
	}

	// BitOffset (of RecordItem): should be 4
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].BitOffset,
		4) {
		t.Error()
	}
	// BitOffset (of RecordItem): should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].BitOffset).Kind(),
		reflect.Int) {
		t.Error()
	}

	// Check correct length of RecordItem[] in Datatype
	if !reflect.DeepEqual(
		len(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray),
		4) {
		t.Error()
	}

	// Set io Device to rexroth
	ioDeviceInterface, ok = ioDeviceMap.Load(ioddFilemapKey_rexroth)
	if !ok {
		t.Error("ioDeviceInterface not found")
	}
	ioDevice, ok = ioDeviceInterface.(IoDevice)
	if !ok {
		t.Error("ioDevice is not of type IoDevice")
	}
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

	// Check correct length of Text[] in ExternalTextCollection>PrimaryLanguage
	if !reflect.DeepEqual(len(ioDevice.ExternalTextCollection.PrimaryLanguage.Text), 98) {
		t.Error()
	}
	//Id: should give out "TI_VendorText"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id, "TI_VendorText") {
		t.Error()
	}
	//Id: type should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id).Kind(),
		reflect.String) {
		t.Error()
	}

	//Value: should give out "www.boschrexroth.com"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value, "www.boschrexroth.com") {
		t.Error()
	}
	//Value: type should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value).Kind(),
		reflect.String) {
		t.Error()
	}

	// bitLength (Datatype): should give out 16
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength,
		uint(16)) {
		t.Error()
	}
	// bitLength (Datatype): should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength).Kind(),
		reflect.Uint) {
		t.Error()
	}

	// BitLength (of SimpleDatatype): should be 0 here (zero because not given/specified in IODD file)
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength,
		uint(0)) {
		t.Error()
	}
	// BitLength (of SimpleDatatype): type should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength).Kind(),
		reflect.Uint) {
		t.Error()
	}

	//xsi:type (of SimpleDatatype): should be BooleanT
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.Type,
		"BooleanT") {
		t.Error()
	}
	//xsi:type (of SimpleDatatype): should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.Type).Kind(),
		reflect.String) {
		t.Error()
	}

	// TextId (of RecordItem>Name): should be DT_RI_Name3640Errorbit
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].Name.TextId,
		"DT_RI_Name3640Errorbit") {
		t.Error()
	}
	// TextId (of RecordItem>Name): should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].Name.TextId).Kind(),
		reflect.String) {
		t.Error()
	}

	// BitOffset (of RecordItem): should be 1
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].BitOffset,
		1) {
		t.Error()
	}
	// BitOffset (of RecordItem): should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].BitOffset).Kind(),
		reflect.Int) {
		t.Error()
	}

	// Check correct length of RecordItem[] in Datatype
	if !reflect.DeepEqual(
		len(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray),
		3) {
		t.Error()
	}

	// Set io Device to siemens
	ioDeviceInterface, ok = ioDeviceMap.Load(ioddFilemapKey_siemens)
	if !ok {
		t.Error("ioDevice is not of type IoDevice")
	}
	ioDevice, ok = ioDeviceInterface.(IoDevice)
	if !ok {
		t.Error("ioDevice is not of type IoDevice")
	}
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

	// Check correct length of Text[] in ExternalTextCollection>PrimaryLanguage
	if !reflect.DeepEqual(len(ioDevice.ExternalTextCollection.PrimaryLanguage.Text), 137) {
		fmt.Printf("ExternalTextCollection length: %v", len(ioDevice.ExternalTextCollection.PrimaryLanguage.Text))
		t.Error()
	}
	//Id: should give out "TI_VendorText"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id, "TI_VendorText") {
		t.Error()
	}
	//Id: type should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id).Kind(),
		reflect.String) {
		t.Error()
	}

	//Value: should give out "Siemens AG"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value, "Siemens AG") {
		t.Error()
	}
	//Value: type should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value).Kind(),
		reflect.String) {
		t.Error()
	}

	// bitLength (Datatype): should give out 16
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength,
		uint(16)) {
		t.Error()
	}
	// bitLength (Datatype): should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength).Kind(),
		reflect.Uint) {
		t.Error()
	}

	// BitLength (of SimpleDatatype): should be 0 here (zero because not given/specified in IODD file)
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength,
		uint(0)) {
		t.Error()
	}
	// BitLength (of SimpleDatatype): type should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength).Kind(),
		reflect.Uint) {
		t.Error()
	}

	//xsi:type (of SimpleDatatype): should be "" (because of different structure: without SimpleDatatype)
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.Type,
		"") {
		t.Error()
	}
	//xsi:type (of SimpleDatatype): should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.Type).Kind(),
		reflect.String) {
		t.Error()
	}

	// TextId (of RecordItem>Name): should be TI_PaeError
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].Name.TextId,
		"TI_PaeError") {
		t.Error()
	}
	// TextId (of RecordItem>Name): should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].Name.TextId).Kind(),
		reflect.String) {
		t.Error()
	}

	// BitOffset (of RecordItem): should be 9
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].BitOffset,
		9) {
		t.Error()
	}
	// BitOffset (of RecordItem): should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].BitOffset).Kind(),
		reflect.Int) {
		t.Error()
	}

	// Check correct length of RecordItem[] in Datatype
	if !reflect.DeepEqual(
		len(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray),
		16) {
		t.Error()
	}

	// Set io Device to rexroth
	ioDeviceInterface, ok = ioDeviceMap.Load(ioddFilemapKey_IFMiodd)
	if !ok {
		t.Error()
	}
	ioDevice, ok = ioDeviceInterface.(IoDevice)
	if !ok {
		t.Error()
	}
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

	// Check correct length of Text[] in ExternalTextCollection>PrimaryLanguage
	if !reflect.DeepEqual(len(ioDevice.ExternalTextCollection.PrimaryLanguage.Text), 144) {
		t.Error()
	}
	//Id: should give out "TI_ProductName0"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id, "TI_ProductName0") {
		t.Error()
	}
	//Id: type should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Id).Kind(),
		reflect.String) {
		t.Error()
	}

	//Value: should give out "DTI410"
	if !reflect.DeepEqual(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value, "DTI410") {
		t.Error()
	}
	//Value: type should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ExternalTextCollection.PrimaryLanguage.Text[0].Value).Kind(),
		reflect.String) {
		t.Error()
	}

	// bitLength (Datatype): should give out 256
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength,
		uint(256)) {
		t.Error()
	}
	// bitLength (Datatype): should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.BitLength).Kind(),
		reflect.Uint) {
		t.Error()
	}

	// BitLength (of SimpleDatatype): should be 0 here (because not specified)
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength,
		uint(0)) {
		t.Errorf(
			"Bitlength was %v and not 0.",
			ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength)
	}
	// BitLength (of SimpleDatatype): type should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength).Kind(),
		reflect.Uint) {
		t.Errorf(
			"Type was %v and not int.",
			reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.BitLength).Kind())
	}

	//xsi:type (of SimpleDatatype): should be BooleanT
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.Type,
		"BooleanT") {
		t.Error()
	}
	//xsi:type (of SimpleDatatype): should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].SimpleDatatype.Type).Kind(),
		reflect.String) {
		t.Error()
	}

	// TextId (of RecordItem>Name): should be TI_PD_SV_IN_2_Name
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].Name.TextId,
		"TI_PD_SV_IN_2_Name") {
		t.Error()
	}
	// TextId (of RecordItem>Name): should be string
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].Name.TextId).Kind(),
		reflect.String) {
		t.Error()
	}

	// BitOffset (of RecordItem): should be 243
	if !reflect.DeepEqual(
		ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].BitOffset,
		243) {
		t.Error()
	}
	// BitOffset (of RecordItem): should be int
	if !reflect.DeepEqual(
		reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray[1].BitOffset).Kind(),
		reflect.Int) {
		t.Error()
	}

	// Check correct length of RecordItem[] in Datatype
	if !reflect.DeepEqual(
		len(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.RecordItemArray),
		8) {
		t.Error()
	}
}
