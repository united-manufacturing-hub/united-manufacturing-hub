package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
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

func GetIoDevice(vendorId int64, deviceId int) (ioDevice IoDevice, err error) {
	//ioddFilemapKey := IoddFilemapKey{VendorId: vendorId, DeviceId: deviceId}

	filemap, err := internal.GetIoddFile(vendorId, deviceId)
	if err != nil {
		return
	}
	var selectedFileFromFilemap []uint8 = filemap[0].File
	fmt.Println("Selected file: " + filemap[0].Name)
	ioDevice, err = UnmarshalIoddFile(selectedFileFromFilemap)
	return
}

// Stores a filemap on harddrive
func CacheFilemap(ioddFilemapKey IoddFilemapKey, filemap []byte) {
	//use gob !!!

	file, err := os.Create(fmt.Sprintf("%d-%d.txt", ioddFilemapKey.VendorId, ioddFilemapKey.DeviceId)) // create/truncate the file
	if err != nil {
		panic(err)
	} // panic if error
	defer file.Close() // make sure it gets closed after
	fmt.Fprintf(file, filemap)

	//copied from https://stackoverflow.com/questions/32687985/convert-back-byte-array-into-file-using-golang
	permissions := 0644 // or whatever you need
	byteArray := []byte("to be written to a file\n")
	err := ioutil.WriteFile("file.txt", byteArray, permissions)
	if err != nil {
		// handle error
	}
}

// Trys to retrieve Filemap from harddrive
func GetFilemapFromCache(ioddFilemapKey IoddFilemapKey) ([]internal.IoDDFile, bool, error) {

	var filemap []internal.IoDDFile
	//Read File
	absolutePath, _ := filepath.Abs(fmt.Sprintf("../sensorconnect/%d-%d.txt", ioddFilemapKey.VendorId, ioddFilemapKey.DeviceId))
	filemapByte, err := ioutil.ReadFile(absolutePath)

	filemap = filemapByte
	if err != nil {
		fmt.Println(err)
		fmt.Println(filemap)
	}

	return
}
