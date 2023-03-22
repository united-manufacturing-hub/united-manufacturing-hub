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
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

// Parsing of Iodd File content
type IoDevice struct {
	DocumentInfo           DocumentInfo           `xml:"DocumentInfo"`
	ExternalTextCollection ExternalTextCollection `xml:"ExternalTextCollection"`
	ProfileBody            ProfileBody            `xml:"ProfileBody"`
}

type DocumentInfo struct {
	ReleaseDate string `xml:"releaseDate,attr"`
}

type ProfileBody struct {
	DeviceIdentity DeviceIdentity `xml:"DeviceIdentity"`
	DeviceFunction DeviceFunction `xml:"DeviceFunction"`
}

type DeviceIdentity struct {
	VendorName string `xml:"vendorName,attr"`
	DeviceId   int    `xml:"deviceId,attr"` // Id of type of a device, given by device vendor
	VendorId   int64  `xml:"vendorId,attr"`
}

type ExternalTextCollection struct {
	PrimaryLanguage PrimaryLanguage `xml:"PrimaryLanguage"`
}

type PrimaryLanguage struct {
	Text []Text `xml:"Text"`
}

type Text struct {
	Id    string `xml:"id,attr"`
	Value string `xml:"value,attr"`
}

type DeviceFunction struct {
	DatatypeCollection    DatatypeCollection    `xml:"DatatypeCollection"`
	ProcessDataCollection ProcessDataCollection `xml:"ProcessDataCollection"`
}

type DatatypeCollection struct {
	DatatypeArray []Datatype `xml:"Datatype"`
}

type ProcessDataCollection struct {
	ProcessData ProcessData `xml:"ProcessData"`
}

type ProcessData struct {
	ProcessDataIn ProcessDataIn `xml:"ProcessDataIn"`
}

type ProcessDataIn struct {
	DatatypeRef DatatypeRef `xml:"DatatypeRef"`
	Name        Name        `xml:"Name"`
	Datatype    Datatype    `xml:"Datatype"`
}

type Datatype struct {
	Type             string        `xml:"type,attr"`
	Id               string        `xml:"id,attr"`
	RecordItemArray  []RecordItem  `xml:"RecordItem"`
	SingleValueArray []SingleValue `xml:"SingleValue"`
	ValueRangeArray  []ValueRange  `xml:"ValueRange"`
	BitLength        uint          `xml:"bitLength,attr"`
	FixedLength      uint          `xml:"fixedLength,attr"`
}

type SingleValue struct {
	Value string `xml:"value,attr"` // can be any kind of type depending on Type of item -> determine later
	Name  Name   `xml:"Name"`
}

type ValueRange struct {
	LowerValue string `xml:"lowerValue,attr"` // can be any kind of type depending on Type of item -> determine later
	UpperValue string `xml:"upperValue,attr"` // can be any kind of type depending on Type of item -> determine later
	Name       Name   `xml:"Name"`
}

type RecordItem struct {
	Name           Name           `xml:"Name"`
	DatatypeRef    DatatypeRef    `xml:"DatatypeRef"`
	SimpleDatatype SimpleDatatype `xml:"SimpleDatatype"`
	BitOffset      int            `xml:"bitOffset,attr"`
}

type Name struct {
	TextId string `xml:"textId,attr"`
}

type DatatypeRef struct {
	DatatypeId string `xml:"datatypeId,attr"`
}

type SimpleDatatype struct {
	ValueRange  ValueRange  `xml:"ValueRange"`
	SingleValue SingleValue `xml:"SingleValue"`
	Type        string      `xml:"type,attr"`
	BitLength   uint        `xml:"bitLength,attr"`
	FixedLength uint        `xml:"fixedLength,attr"`
}

// Todo add datatyperef if not simple datatype

// IoddFilemapKey Further Datastructures
type IoddFilemapKey struct {
	VendorId int64
	DeviceId int
}

// AddNewDeviceToIoddFilesAndMap uses ioddFilemapKey to download new iodd file (if key not alreaddy in IoDevice map).
// Then updates map by checking for new files, unmarhaling them und importing into map
func AddNewDeviceToIoddFilesAndMap(
	ioddFilemapKey IoddFilemapKey,
	relativeDirectoryPath string,
	fileInfoSlice []fs.DirEntry, isTest bool) ([]fs.DirEntry, error) {
	zap.S().Debugf("Requesting IoddFile %v -> %s", ioddFilemapKey, relativeDirectoryPath)
	err := RequestSaveIoddFile(ioddFilemapKey, relativeDirectoryPath, isTest)
	if err != nil {
		zap.S().Debugf("File with fileMapKey%v already saved.", ioddFilemapKey)
	}
	zap.S().Debugf("Reading IoddFiles %v -> %s", fileInfoSlice, relativeDirectoryPath)
	fileInfoSlice, err = ReadIoddFiles(fileInfoSlice, relativeDirectoryPath)
	if err != nil {
		zap.S().Errorf("Error in AddNewDeviceToIoddFilesAndMap: %s", err.Error())
		return fileInfoSlice, err
	}
	return fileInfoSlice, nil
}

func UnmarshalIoddFile(ioddFile []byte, absoluteFilePath string) (IoDevice, error) {
	payload := IoDevice{}

	// Unmarshal file with Unmarshal
	err := xml.Unmarshal(ioddFile, &payload)
	if err != nil {
		zap.S().Errorf(
			"Unmarshaling of IoDevice %#v failed. Deleting iodd.xml file now and stopping container after that. Error: %v",
			ioddFile,
			err)
		err = os.Remove(absoluteFilePath)
		// If unmarshaling fails we remove the iodd.xml file and stop tze container
		if err != nil {
			zap.S().Errorf("Removing file: %#v failed. Stopping container now. Error: %v", absoluteFilePath, err)
		}
		zap.S().Fatalf("Failed to unmarshal iodd.xml file. Stopping container now. Error: %v", err)
	}
	return payload, err
}

// ReadIoddFiles Determines with oldFileInfoSlice if new .xml Iodd files are in IoddFiles folder -> if yes: unmarshals new files and caches in IoDevice Map
func ReadIoddFiles(oldFileInfoSlice []fs.DirEntry, relativeDirectoryPath string) ([]fs.DirEntry, error) {
	absoluteDirectoryPath, err := filepath.Abs(relativeDirectoryPath)
	if err != nil {
		zap.S().Errorf("Error in ReadIoddFiles: %s", err.Error())
		return oldFileInfoSlice, err
	}
	// check for new iodd files
	currentFileInfoSlice, err := os.ReadDir(absoluteDirectoryPath)
	if err != nil {
		err = errors.New("reading of currentFileInfoSlice from specified directory failed")
		return oldFileInfoSlice, err
	}
	currentNames := getNamesOfFileInfo(currentFileInfoSlice)
	oldNames := getNamesOfFileInfo(oldFileInfoSlice)

	for _, name := range currentNames {
		if contains(oldNames, name) {
			continue
		}
		zap.S().Debugf("File %v not already in map.", name)
		// if the oldFileInfoSlice doesn't contain a file with this name the file is new
		// create path to file
		absoluteFilePath := absoluteDirectoryPath + "/" + name
		// read file
		var dat []byte
		dat, err = os.ReadFile(absoluteFilePath)
		if err != nil {
			return oldFileInfoSlice, err
		}
		// Unmarshal
		var ioDevice IoDevice
		ioDevice, err = UnmarshalIoddFile(dat, absoluteFilePath)
		if err != nil {
			return oldFileInfoSlice, err
		}
		// create ioddFilemapKey of unmarshaled file
		var ioddFilemapKey IoddFilemapKey
		ioddFilemapKey.DeviceId = ioDevice.ProfileBody.DeviceIdentity.DeviceId
		ioddFilemapKey.VendorId = ioDevice.ProfileBody.DeviceIdentity.VendorId
		// Check if IoDevice for created ioddfilemapKey already exists in ioddIoDeviceMap

		if idm, ok := ioDeviceMap.Load(ioddFilemapKey); ok {
			// IoDevice is already in ioddIoDeviceMap
			// -> replace depending on date (the newest version should be used)
			var earlier bool
			if earlier, err = currentDateEarlierThenOldDate(
				ioDevice.DocumentInfo.ReleaseDate,
				idm.(IoDevice).DocumentInfo.ReleaseDate); earlier && err == nil {
				ioDeviceMap.Store(ioddFilemapKey, ioDevice)
			}
			continue
		}
		ioDeviceMap.Store(ioddFilemapKey, ioDevice)
	}
	return currentFileInfoSlice, err
}

// RequestSaveIoddFile will download iodd file if the ioddFilemapKey is not already in ioddIoDeviceMap
func RequestSaveIoddFile(ioddFilemapKey IoddFilemapKey, relativeDirectoryPath string, isTest bool) error {
	var err error
	// Check if IoDevice already in ioddIoDeviceMap
	if _, ok := ioDeviceMap.Load(ioddFilemapKey); ok {
		err = errors.New("request to save Iodd File invalid: entry for ioddFilemapKey already exists")
		return err
	}
	// Execute download and saving of iodd file
	err = internal.SaveIoddFile(ioddFilemapKey.VendorId, ioddFilemapKey.DeviceId, relativeDirectoryPath, isTest)
	if err != nil {
		zap.S().Errorf("Saving error: %s", err.Error())
		return err
	}
	return nil
}

// Checks if slice contains entry
func contains(slice []string, entry string) bool {
	for _, element := range slice {
		if element == entry {
			return true
		}
	}
	return false
}

// getNamesOfFileInfo returns the names of files stored inside a FileInfo slice
func getNamesOfFileInfo(fileInfoSlice []fs.DirEntry) (namesSlice []string) {
	for _, element := range fileInfoSlice {
		namesSlice = append(namesSlice, element.Name())
	}
	return
}

// currentDateEarlierThenOldDate returns if a date is earlier then another
func currentDateEarlierThenOldDate(currentDate string, oldDate string) (bool, error) {
	const shortDate = "2006-01-02"
	parsedCurrentDate, err := time.Parse(shortDate, currentDate)
	if err != nil {
		return true, err // Defaults to true so entries in IoDevice Map are not changed
	}
	parsedOldDate, err := time.Parse(shortDate, oldDate)
	if err != nil {
		return true, err // Defaults to true so entries in IoDevice Map are not changed
	}

	if parsedCurrentDate.After(parsedOldDate) {
		return false, err
	}
	return true, err
}
