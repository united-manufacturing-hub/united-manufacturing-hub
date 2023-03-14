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

package main

import jsoniter "github.com/json-iterator/go"

func UnmarshalRawImage(data []byte) (RawImage, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	var r RawImage
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *RawImage) Marshal() ([]byte, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	return json.Marshal(r)
}

type RawImage struct {
	ImageID       string `json:"image_id"`
	ImageBytes    string `json:"image_bytes"`
	ImageHeight   int64  `json:"image_height"`
	ImageWidth    int64  `json:"image_width"`
	ImageChannels int64  `json:"image_channels"`
}
