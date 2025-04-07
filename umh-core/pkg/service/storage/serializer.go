/*
 *   Copyright (c) 2025 UMH Systems GmbH
 *   All rights reserved.

 *   Licensed under the Apache License, Version 2.0 (the "License");
 *   you may not use this file except in compliance with the License.
 *   You may obtain a copy of the License at

 *   http://www.apache.org/licenses/LICENSE-2.0

 *   Unless required by applicable law or agreed to in writing, software
 *   distributed under the License is distributed on an "AS IS" BASIS,
 *   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *   See the License for the specific language governing permissions and
 *   limitations under the License.
 */

package storage

import (
	"bytes"
	"errors"

	"gopkg.in/yaml.v3"
)

// Serializer is an interface that defines the methods for encoding and decoding objects for the storage layer
type Serializer interface {
	// Encode encodes an object into a byte slice
	Encode(object Object) ([]byte, error)
	// Decode decodes a byte slice into an object
	Decode(data []byte, objType Object) error
}

// YamlSerializer is a serializer that uses yaml to encode and decode objects
type YamlSerializer struct{}

func NewYamlSerializer() *YamlSerializer {
	return &YamlSerializer{}
}

// Encode encodes an object into a byte slice
func (s *YamlSerializer) Encode(object Object) ([]byte, error) {
	if object == nil {
		return nil, errors.New("object cannot be nil")
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	if err := encoder.Encode(object); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Decode decodes a byte slice into an object
func (s *YamlSerializer) Decode(data []byte, obj Object) error {
	if obj == nil {
		return errors.New("object type cannot be nil")
	}

	if len(data) == 0 {
		return errors.New("data cannot be empty")
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(obj); err != nil {
		return err
	}

	return nil
}
