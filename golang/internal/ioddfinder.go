package internal

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

func GetIoddFile(vendorId int64, deviceId int) (files []IoDDFile, err error) {
	var body []byte
	body, err = getUrl(fmt.Sprintf("https://ioddfinder.io-link.com/api/drivers?page=0&size=2000&status=APPROVED&status=UPLOADED&deviceIdString=%d", deviceId))
	if err != nil {
		return
	}
	var ioddfinder Ioddfinder
	ioddfinder, err = UnmarshalIoddfinder(body)
	if err != nil {
		return
	}

	validIds := make([]int, 0)

	for i, content := range ioddfinder.Content {
		if content.VendorID == vendorId {
			validIds = append(validIds, i)
		}
	}

	if len(validIds) == 0 {
		err = errors.New(fmt.Sprintf("No IODD file for vendorID [%d] and deviceID [%d]", vendorId, deviceId))
		return
	}

	files = make([]IoDDFile, 0)

	for _, id := range validIds {
		ioddId := ioddfinder.Content[id].IoddID
		var ioddzip []byte
		ioddzip, err = getUrl(fmt.Sprintf("https://ioddfinder.io-link.com/api/vendors/%d/iodds/%d/files/zip/rated", vendorId, ioddId))
		if err != nil {
			return
		}
		var zipReader *zip.Reader
		zipReader, err = zip.NewReader(bytes.NewReader(ioddzip), int64(len(ioddzip)))
		if err != nil {
			return
		}

		for _, zipFile := range zipReader.File {
			if strings.HasSuffix(zipFile.Name, "xml") {
				var file []byte
				file, err = readZipFile(zipFile)
				if err != nil {
					return
				}
				files = append(files, IoDDFile{
					Name:    zipFile.Name,
					File:    file,
					Context: ioddfinder.Content[id],
				})
			}
		}
	}

	return
}

type IoDDFile struct {
	Name    string
	File    []byte
	Context Content
}

func readZipFile(zf *zip.File) ([]byte, error) {
	f, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}

func getUrl(url string) (body []byte, err error) {
	var resp *http.Response
	resp, err = http.Get(url)
	defer resp.Body.Close()
	if err != nil {
		return
	}
	body, err = ioutil.ReadAll(resp.Body)
	return
}

func UnmarshalIoddfinder(data []byte) (Ioddfinder, error) {
	var r Ioddfinder
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *Ioddfinder) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type Ioddfinder struct {
	Content          []Content     `json:"content"`
	Number           int64         `json:"number"`
	Size             int64         `json:"size"`
	NumberOfElements int64         `json:"numberOfElements"`
	Sort             []interface{} `json:"sort"`
	First            bool          `json:"first"`
	Last             bool          `json:"last"`
	TotalPages       int64         `json:"totalPages"`
	TotalElements    int64         `json:"totalElements"`
}

type Content struct {
	HasMoreVersions    bool   `json:"hasMoreVersions"`
	DeviceID           int64  `json:"deviceId"`
	IoLinkRev          string `json:"ioLinkRev"`
	VersionString      string `json:"versionString"`
	IoddID             int64  `json:"ioddId"`
	ProductID          string `json:"productId"`
	ProductVariantID   int64  `json:"productVariantId"`
	ProductName        string `json:"productName"`
	VendorName         string `json:"vendorName"`
	UploadDate         int64  `json:"uploadDate"`
	VendorID           int64  `json:"vendorId"`
	IoddStatus         string `json:"ioddStatus"`
	IndicationOfSource string `json:"indicationOfSource"`
}
