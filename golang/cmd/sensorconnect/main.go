package main

import (
	"fmt"
	"io/ioutil"
	"reflect" //for reading out type of variable
	/*
		"go.uber.org/zap" */)

func main() {
	//Read File
	dat, err := ioutil.ReadFile("C:/Users/LarsWitte/umh_larswitte_repo/sensorconnectRep/united-manufacturing-hub/golang/cmd/sensorconnect/ifm-0002BA-20170227-IODD1.1.xml")
	if err != nil {
		panic(err)
	}
	fmt.Println("Contents of file:", string(dat))

	// Unmarshal file
	var ioDevice IoDevice
	ioDevice, err = UnmarshalIoddFile(dat)
	if err != nil {
		panic(err)
	}
	/* 	if err != nil {
	   		zap.S().Warnf("Invalid IoddFile: %s", err)
	   	}
	*/
	//should give out 698 and int
	fmt.Println(ioDevice.ProfileBody.DeviceIdentity.DeviceId)
	fmt.Println(reflect.TypeOf(ioDevice.ProfileBody.DeviceIdentity.DeviceId))

	//should put out 4 and int
	fmt.Println(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength)
	fmt.Println(reflect.TypeOf(ioDevice.ProfileBody.DeviceFunction.ProcessDataCollection.ProcessData.ProcessDataIn.Datatype.ReccordItem[1].SimpleDatatype.BitLength))
	fmt.Println(reflect.TypeOf(dat))
}
