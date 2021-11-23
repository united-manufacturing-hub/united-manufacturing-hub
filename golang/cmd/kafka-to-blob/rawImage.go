package main

import "encoding/json"

func UnmarshalRawImage(data []byte) (RawImage, error) {
	var r RawImage
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *RawImage) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type RawImage struct {
	ImageID       string `json:"image_id"`
	ImageBytes    string `json:"image_bytes"`
	ImageHeight   int64  `json:"image_height"`
	ImageWidth    int64  `json:"image_width"`
	ImageChannels int64  `json:"image_channels"`
}
