package main

import (
	"encoding/xml"
	"errors"
	"io/ioutil"
	"path/filepath"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
)

// Parsing of Iodd File content
type IoDevice struct {
	ProfileBody            ProfileBody            `xml:"ProfileBody"`
	ExternalTextCollection ExternalTextCollection `xml:"ExternalTextCollection"`
}

type ProfileBody struct {
	DeviceIdentity DeviceIdentity `xml:"DeviceIdentity"`
	DeviceFunction DeviceFunction `xml:"DeviceFunction"`
}

type DeviceIdentity struct {
	VendorName string `xml:"vendorName,attr"`
	DeviceId   int    `xml:"deviceId,attr"` // Id of type of a device, given by device vendor
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

func UnmarshalIoddFile(ioddFile []uint8) (IoDevice, error) {
	payload := IoDevice{}

	// Unmarshal file with Unmarshal
	err := xml.Unmarshal(ioddFile, &payload)
	if err != nil {
		panic(err) //Todo change to zap stuff
	}
	return payload, err
}

func ReadIoddFiles() {
	// check for new iodd files

	// unmarshal new iodd files + check if already in ioDevice map

	// if already in io device map: use newest version
	absolutePath, _ := filepath.Abs("../sensorconnect/ifm-0002BA-20170227-IODD1.1.xml")
	dat, err := ioutil.ReadFile(absolutePath)
	if err != nil {
		panic(err)
	}
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
