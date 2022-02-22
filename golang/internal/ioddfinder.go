package internal

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

func SaveIoddFile(vendorId int64, deviceId int, relativeDirectoryPath string) (err error) {
	// download iodd file
	filemap, err := GetIoddFile(vendorId, deviceId)
	if err != nil {
		return err
	}

	// build path for downloaded file
	absoluteDirectoryPath, err := filepath.Abs(relativeDirectoryPath)
	if err != nil {
		zap.S().Errorf("Unable to find absoluteDirectoryPath: %v", err)
		return err
	}

	//Get latest file
	latest := int64(0)
	index := 0
	for i, file := range filemap {
		if file.Context.UploadDate > latest {
			index = i
			latest = file.Context.UploadDate
		}
	}

	absoluteFilePath := absoluteDirectoryPath + "/" + filemap[index].Name
	zap.S().Debugf("Saving file to path: " + absoluteFilePath)

	// check for existing file with same name
	if _, err := os.Stat(absoluteFilePath); err == nil {
		return nil
	}

	// save iodd file
	err = ioutil.WriteFile(absoluteFilePath, filemap[index].File, 0644)
	if err != nil {
		zap.S().Errorf("Unable to write file: %v", err)
		return err
	}
	return
}

// GetIoddFile downloads a ioddfiles from ioddfinder and returns a list of valid files for the request (This can be multiple, if the vendor has multiple languages or versions published)
func GetIoddFile(vendorId int64, deviceId int) (files []IoDDFile, err error) {
	var body []byte
	body, err = getUrlWithRetry(fmt.Sprintf("https://ioddfinder.io-link.com/api/drivers?page=0&size=2000&status=APPROVED&status=UPLOADED&deviceIdString=%d", deviceId))
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
		ioddzip, err = getUrlWithRetry(fmt.Sprintf("https://ioddfinder.io-link.com/api/vendors/%d/iodds/%d/files/zip/rated", vendorId, ioddId))
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

// IoDDFile is a helper structure with the name, file and additional context of the iodd file
type IoDDFile struct {
	Name    string
	File    []byte
	Context Content
}

// readZipFile gets the content of a zip file
func readZipFile(zf *zip.File) ([]byte, error) {
	f, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}

func getUrlWithRetry(url string) (body []byte, err error) {
	cacheKey := fmt.Sprintf("getUrlWithRetry%s", url)

	val, found := GetMemcached(cacheKey)
	if found {
		return val.([]byte), nil
	}

	zap.S().Debugf("Getting url %s", url)
	var status int
	status = 1
	for i := 0; i < 10; i++ {
		body, err, status = getUrl(url)
		if err != nil {
			return
		}
		if status == 200 {
			SetMemcached(cacheKey, body)
			return
		}
		time.Sleep(GetBackoffTime(int64(i), 10*time.Second, 60*time.Second))
	}
	err = errors.New("failed to retrieve url after 10 tries")
	return
}

var globalSleepTimer = 0

// getUrl executes a GET request to an url and returns the body as bytes
func getUrl(url string) (body []byte, err error, status int) {
	time.Sleep(GetBackoffTime(int64(globalSleepTimer), 10*time.Millisecond, 1*time.Second))
	globalSleepTimer += 1
	var resp *http.Response
	resp, err = http.Get(url)
	if err != nil {
		zap.S().Errorf("%s", err.Error())
		return
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	if status != 200 {
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
