// Copyright 2025 UMH Systems GmbH
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

package ptr

func TruePtr() *bool {
	return &[]bool{true}[0]
}

func FalsePtr() *bool {
	return &[]bool{false}[0]
}

func Uint8Ptr(i uint8) *uint8 {
	return &i
}

func Uint16Ptr(i uint16) *uint16 {
	return &i
}

func Uint32Ptr(i uint32) *uint32 {
	return &i
}

func Uint64Ptr(i uint64) *uint64 {
	return &i
}

func Int8Ptr(i int8) *int8 {
	return &i
}

func Int16Ptr(i int16) *int16 {
	return &i
}

func Int32Ptr(i int32) *int32 {
	return &i
}

func Int64Ptr(i int64) *int64 {
	return &i
}

func Float32Ptr(i float32) *float32 {
	return &i
}

func Float64Ptr(i float64) *float64 {
	return &i
}

func Complex64Ptr(i complex64) *complex64 {
	return &i
}

func Complex128Ptr(i complex128) *complex128 {
	return &i
}

func StringPtr(s string) *string {
	return &s
}

func IntPtr(i int) *int {
	return &i
}

func UintPtr(i uint) *uint {
	return &i
}

func UintptrPtr(i uintptr) *uintptr {
	return &i
}

func BytePtr(b byte) *byte {
	return &b
}

func RunePtr(r rune) *rune {
	return &r
}
