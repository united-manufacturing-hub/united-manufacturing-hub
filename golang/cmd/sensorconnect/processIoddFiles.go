package main

import (
	"encoding/xml"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

// Parsing of Iodd File content
type IoDevice struct {
	ProfileBody            ProfileBody            `xml:"ProfileBody"`
	ExternalTextCollection ExternalTextCollection `xml:"ExternalTextCollection"`
	DocumentInfo           DocumentInfo           `xml:"DocumentInfo"`
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
	ProcessDataCollection ProcessDataCollection `xml:"ProcessDataCollection"` //ToDo: array?
}

type ProcessDataCollection struct {
	ProcessData ProcessData `xml:"ProcessData"`
}

type ProcessData struct {
	ProcessDataIn ProcessDataIn `xml:"ProcessDataIn"`
}

type ProcessDataIn struct {
	Datatype Datatype
}

type Datatype struct {
	BitLength   int          `xml:"bitLength,attr"`
	ReccordItem []RecordItem `xml:"RecordItem"`
}

type RecordItem struct {
	BitOffset      int            `xml:"bitOffset,attr"`
	SimpleDatatype SimpleDatatype `xml:"SimpleDatatype"`
	Name           Name           `xml:"Name"`
}

type Name struct {
	TextId string `xml:"textId,attr"`
}

type SimpleDatatype struct {
	Type        string `xml:"type,attr"` // Dropped "xsi:" to correctly unmarshal
	BitLength   int    `xml:"bitLength,attr"`
	FixedLength int    `xml:"fixedLength,attr"`
}

//Further Datastructures
type IoddFilemapKey struct {
	VendorId int64
	DeviceId int
}

// uses ioddFilemapKey to download new iodd file (if key not alreaddy in IoDevice map).
// Then updates map by checking for new files, unmarhaling them und importing into map
func AddNewDeviceToIoddFilesAndMap(ioddFilemapKey IoddFilemapKey, relativeDirectoryPath string, ioddIoDeviceMap map[IoddFilemapKey]IoDevice, fileInfoSlice []os.FileInfo) (map[IoddFilemapKey]IoDevice, []os.FileInfo, error) {
	err := RequestSaveIoddFile(ioddFilemapKey, ioddIoDeviceMap, relativeDirectoryPath)
	if err != nil {
		return ioddIoDeviceMap, fileInfoSlice, err
	}
	ioddIoDeviceMap, fileInfoSlice, err = ReadIoddFiles(ioddIoDeviceMap, fileInfoSlice, relativeDirectoryPath)
	if err != nil {
		return ioddIoDeviceMap, fileInfoSlice, err
	}
	return ioddIoDeviceMap, fileInfoSlice, err
}

func UnmarshalIoddFile(ioddFile []byte) (IoDevice, error) {
	payload := IoDevice{}

	// Unmarshal file with Unmarshal
	err := xml.Unmarshal(ioddFile, &payload)
	if err != nil {
		panic(err) //Todo change to zap stuff
	}
	return payload, err
}

// Determines with oldFileInfoSlice if new .xml Iodd files are in IoddFiles folder -> if yes: unmarshals new files and caches in IoDevice Map
func ReadIoddFiles(ioddIoDeviceMap map[IoddFilemapKey]IoDevice, oldFileInfoSlice []os.FileInfo, relativeDirectoryPath string) (map[IoddFilemapKey]IoDevice, []os.FileInfo, error) {
	absoluteDirectoryPath, _ := filepath.Abs(relativeDirectoryPath)
	// check for new iodd files
	currentFileInfoSlice, err := ioutil.ReadDir(absoluteDirectoryPath)
	if err != nil {
		err = errors.New("reading of currentFileInfoSlice from specified directory failed")
		return ioddIoDeviceMap, oldFileInfoSlice, err
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
		absoluteFilePath := absoluteDirectoryPath + "\\" + name
		// read file
		dat, err := ioutil.ReadFile(absoluteFilePath)
		if err != nil {
			return ioddIoDeviceMap, oldFileInfoSlice, err
		}
		// Unmarshal
		ioDevice := IoDevice{}
		ioDevice, err = UnmarshalIoddFile(dat)
		if err != nil {
			return ioddIoDeviceMap, oldFileInfoSlice, err
		}
		// create ioddFilemapKey of unmarshaled file
		var ioddFilemapKey IoddFilemapKey
		ioddFilemapKey.DeviceId = ioDevice.ProfileBody.DeviceIdentity.DeviceId
		ioddFilemapKey.VendorId = ioDevice.ProfileBody.DeviceIdentity.VendorId
		// Check if IoDevice for created ioddfilemapKey already exists in ioddIoDeviceMap
		if _, ok := ioddIoDeviceMap[ioddFilemapKey]; ok {
			// IoDevice is already in ioddIoDeviceMap
			// -> replace depending on date (newest version should be used)
			if earlier, _ := currentDateEarlierThenOldDate(ioDevice.DocumentInfo.ReleaseDate, ioddIoDeviceMap[ioddFilemapKey].DocumentInfo.ReleaseDate); earlier {
				ioddIoDeviceMap[ioddFilemapKey] = ioDevice
			}
			continue
		}
		ioddIoDeviceMap[ioddFilemapKey] = ioDevice
	}
	return ioddIoDeviceMap, currentFileInfoSlice, err
}

// will download iodd file if the ioddFilemapKey is not already in ioddIoDeviceMap
func RequestSaveIoddFile(ioddFilemapKey IoddFilemapKey, ioddIoDeviceMap map[IoddFilemapKey]IoDevice, relativeDirectoryPath string) error {
	var err error
	// Check if IoDevice already in ioddIoDeviceMap
	if _, ok := ioddIoDeviceMap[ioddFilemapKey]; ok {
		err = errors.New("request to save Iodd File invalid: entry for ioddFilemapKey already exists")
		return err
	}
	// Execute download and saving of iodd file
	err = internal.SaveIoddFile(ioddFilemapKey.VendorId, ioddFilemapKey.DeviceId, relativeDirectoryPath)
	if err != nil {
		return err
	}
	return err
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

func getNamesOfFileInfo(fileInfoSlice []os.FileInfo) (namesSlice []string) {
	for _, element := range fileInfoSlice {
		namesSlice = append(namesSlice, element.Name())
	}
	return
}

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
