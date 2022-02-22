package internal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestGetIoddFile(t *testing.T) {
	//Siemens AG | SIRIUS ACT Electronic Module 4DI/4DQ for IO-Link
	err := AssertIoddFileGetter(42, 278531, 2)
	if err != nil {
		t.Error(err)
	}
	//Bosch Rexroth AG | 4WRPEH10-3X
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
		return errors.New(fmt.Sprintf("Filemap lenght invalid Have: %d Wanted: %d", len(filemap), filemaplen))
	}

	for _, file := range filemap {
		fmt.Println("====")
		fmt.Println(file.Name)
		fmt.Println(file.Context)

	}

	return nil
}

func TestSaveIoddFile(t *testing.T) {
	relativeDirectoryPath := "../cmd/sensorconnect/IoddFiles/"
	// test if writing iodd file works
	err := SaveIoddFile(42, 278531, relativeDirectoryPath)
	if err != nil {
		t.Error(err)
	}

	// test if existing file is detected and corresponding error is thrown
	err = SaveIoddFile(42, 278531, relativeDirectoryPath)
	if err == nil {
		t.Error(err)
	}

	// Remove file after test again
	relativeFilePath := "../cmd/sensorconnect/IoddFiles/Siemens-SIRIUS-3SU1-4DI4DQ-20160602-IODD1.0.1.xml"
	absoluteFilePath, _ := filepath.Abs(relativeFilePath)
	fmt.Println(absoluteFilePath)
	err = os.Remove(absoluteFilePath)
	if err != nil {
		fmt.Println("file not deleted")
		t.Error(err)
	}
}
