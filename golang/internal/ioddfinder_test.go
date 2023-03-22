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

package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestGetIoddFile(t *testing.T) {
	InitCacheWithoutRedis()
	// Siemens AG | SIRIUS ACT Electronic Module 4DI/4DQ for IO-Link
	err := AssertIoddFileGetter(42, 278531, 2)
	if err != nil {
		t.Error(err)
	}
	// Bosch Rexroth AG | 4WRPEH10-3X
	err = AssertIoddFileGetter(287, 2228227, 5)
	if err != nil {
		t.Error(err)
	}
	// ifm electronic gmbh | DTI410
	err = AssertIoddFileGetter(310, 967, 140)
	if err != nil {
		t.Error(err)
	}
}

func AssertIoddFileGetter(vendorId int64, deviceId int, filemaplen int) error {
	filemap, err := GetIoddFile(vendorId, deviceId)
	if err != nil {
		return err
	}

	if len(filemap) != filemaplen {
		return fmt.Errorf("Filemap length invalid Have: %d Wanted: %d", len(filemap), filemaplen)
	}

	return nil
}

func TestSaveIoddFile(t *testing.T) {
	relativeDirectoryPath := "../cmd/sensorconnect/IoddFiles/"
	err := os.MkdirAll(filepath.Dir(relativeDirectoryPath), 0777)
	if err != nil {
		t.Fatal(err)
	}
	// test if writing iodd file works
	err = SaveIoddFile(42, 278531, relativeDirectoryPath, false)
	if err != nil {
		t.Error(err)
	}

	// test if existing file is detected and corresponding error is thrown
	err = SaveIoddFile(42, 278531, relativeDirectoryPath, false)
	if err != nil {
		t.Error(err)
	}

	// Remove file after test again
	relativeFilePath := "../cmd/sensorconnect/IoddFiles/Siemens-SIRIUS-3SU1-4DI4DQ-20160602-IODD1.1.xml"
	var absoluteFilePath string
	absoluteFilePath, err = filepath.Abs(relativeFilePath)
	if err != nil {
		t.Error(err)
	}
	err = os.Remove(absoluteFilePath)
	if err != nil {
		t.Logf("file not deleted: %s", err)
		t.Error(err)
	}
}
