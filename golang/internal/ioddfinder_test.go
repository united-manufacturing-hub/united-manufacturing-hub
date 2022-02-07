package internal

import (
	"errors"
	"fmt"
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
