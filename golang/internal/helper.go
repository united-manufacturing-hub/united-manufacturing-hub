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
	"bytes"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"

	"go.uber.org/zap"
)

var lock2 sync.Mutex

// Marshal is a function that marshals the object into an
// io.Reader.
// By default, it uses the JSON marshaller.
var Marshal = func(v interface{}) (io.Reader, error) {

	b, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// Save saves a representation of v to the file at path.
func Save(path string, v interface{}) error {
	lock2.Lock()
	defer lock2.Unlock()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := Marshal(v)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	return err
}

// Unmarshal is a function that unmarshals the data from the
// reader into the specified value.
// By default, it uses the JSON unmarshaller.
var Unmarshal = func(r io.Reader, v interface{}) error {

	return json.NewDecoder(r).Decode(v)
}

// Load loads the file at path into v.
// Use os.IsNotExist() to see if the returned error is due
// to the file being missing.
func Load(path string, v interface{}) error {
	lock2.Lock()
	defer lock2.Unlock()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return Unmarshal(f, v)
}

// LogObject is used to create testfiles for golden testing
func LogObject(functionName string, objectName string, now time.Time, v interface{}) {
	timestamp := strconv.FormatInt(now.UTC().Unix(), 10)
	err := Save("/testfiles/temp/"+functionName+"_"+objectName+"_"+timestamp+".golden", v)
	if err != nil {
		zap.S().Errorf("Error while logging object: %s", err)
	}

	zap.S().Infof("Logged ", "/testfiles/temp/"+functionName+"_"+objectName+"_"+timestamp+".golden")
}

// UniqueInt returns a unique subset of the int slice provided.
func UniqueInt(input []int) []int { // Source: https://kylewbanks.com/blog/creating-unique-slices-in-go
	u := make([]int, 0, len(input))
	m := make(map[int]bool)

	for _, val := range input {
		if _, ok := m[val]; !ok {
			m[val] = true
			u = append(u, val)
		}
	}

	return u
}

// IsInSliceFloat64 takes a slice and looks for an element in it. If found it will
// return it's key, otherwise it will return -1 and a bool of false.
func IsInSliceFloat64(slice []float64, val float64) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// IsInSliceInt32 takes a slice and looks for an element in it. If found it will
// return it's key, otherwise it will return -1 and a bool of false.
func IsInSliceInt32(slice []int32, val int32) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// Divmod allows division with remainder. Source: https://stackoverflow.com/questions/43945675/division-with-returning-quotient-and-remainder
func Divmod(numerator, denominator int64) (quotient, remainder int64) {
	quotient = numerator / denominator // integer division, decimals are truncated
	remainder = numerator % denominator
	return
}

func IsValidStruct(testStruct interface{}, allowedNilFields []string) (success bool) {
	success = true
	v := reflect.ValueOf(testStruct)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Pointer() == 0 {
			fieldName := v.Type().Field(i).Name
			if contains(allowedNilFields, fieldName) {
				continue
			}
			zap.S().Warnf("%s is nil, check for typing errors !\n", fieldName)
			success = false
		}
	}
	return
}
func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func EnvIsTrue(envVariable string) bool {
	env := strings.ToLower(os.Getenv(envVariable))
	switch env {
	case "true", "1", "yes", "on", "enable", "enabled", "active":
		return true
	}
	return false
}

func IndexOf(slice []string, value string) int {
	for p, v := range slice {
		if v == value {
			return p
		}
	}
	return -1
}
