package main

import (
	"encoding/xml"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
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

func UnmarshalIoddFile(ioddFile []byte) (IoDevice, error) {
	payload := IoDevice{}

	// Unmarshal file with Unmarshal
	err := xml.Unmarshal(ioddFile, &payload)
	if err != nil {
		panic(err) //Todo change to zap stuff
	}
	return payload, err
}

//
func ReadIoddFiles(oldFileInfoSlice []os.FileInfo, ioddIoDeviceMap map[IoddFilemapKey]IoDevice) (map[IoddFilemapKey]IoDevice, []os.FileInfo, error) {
	relativeDirectoryPath := "../cmd/sensorconnect/IoddFiles/"
	absoluteDirectoryPath, _ := filepath.Abs(relativeDirectoryPath)
	// check for new iodd files
	currentFileInfoSlice, err := ioutil.ReadDir(absoluteDirectoryPath)
	if err != nil {
		panic(err)
	}
	currentNames := getNamesOfFileInfo(currentFileInfoSlice)
	oldNames := getNamesOfFileInfo(oldFileInfoSlice)
	for _, name := range currentNames {
		if contains(oldNames, name) {
			continue
		}
		// if the oldFileInfoSlice doesn't contain a file with this name the file is new
		// create path to file
		absoluteFilePath := absoluteDirectoryPath + "\\" + name
		// read file
		dat, err := ioutil.ReadFile(absoluteFilePath)
		if err != nil {
			panic(err)
		}
		// Unmarshal
		ioDevice := IoDevice{}
		ioDevice, err = UnmarshalIoddFile(dat)
		// check if vendorId/deviceId combination already in ioDevice map
		var ioddFilemapKey IoddFilemapKey
		ioddFilemapKey.DeviceId = ioDevice.ProfileBody.DeviceIdentity.DeviceId
		ioddFilemapKey.VendorId = ioDevice.ProfileBody.DeviceIdentity.VendorId
		// Check if IoDevice already in ioddIoDeviceMap
		if _, ok := ioddIoDeviceMap[ioddFilemapKey]; ok {
			// IoDevice is already in ioddIoDeviceMap
			// -> replace depending on date (newest version should be used)
			if earlier, _ := currentDateEarlierThenOldDate(ioDevice.DocumentInfo.ReleaseDate, ioddIoDeviceMap[ioddFilemapKey].DocumentInfo.ReleaseDate); earlier {
				ioddIoDeviceMap[ioddFilemapKey] = ioDevice
			}
			return ioddIoDeviceMap
		}

	}
	return ioddIoDeviceMap
	// if already in io device map: use newest version
}

func RequestSaveIoddFile(ioddFilemapKey IoddFilemapKey, ioddIoDeviceMap map[IoddFilemapKey]IoDevice) (err error) {
	// Check if IoDevice already in ioddIoDeviceMap
	if _, ok := ioddIoDeviceMap[ioddFilemapKey]; ok {
		err := errors.New("request to save Iodd File invalid: entry for ioddFilemapKey already exists")
		return err
	}

	// Execute download and saving of iodd file
	err = internal.SaveIoddFile(ioddFilemapKey.VendorId, ioddFilemapKey.DeviceId)
	if err != nil {
		return err
	}
	return
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
