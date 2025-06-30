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

package encoding

import (
	encoding_corev1 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding/corev1"
	encoding_new "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding/new"
	encoding_old "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding/old"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var encodeMessageFromUserToUMHInstance func(UMHMessage models.UMHMessageContent) (string, error) = encoding_old.EncodeMessageFromUserToUMHInstance
var encodeMessageFromUMHInstanceToUser func(UMHMessage models.UMHMessageContent) (string, error) = encoding_old.EncodeMessageFromUMHInstanceToUser
var decodeMessageFromUserToUMHInstance func(base64Message string) (models.UMHMessageContent, error) = encoding_old.DecodeMessageFromUserToUMHInstance
var decodeMessageFromUMHInstanceToUser func(base64Message string) (models.UMHMessageContent, error) = encoding_old.DecodeMessageFromUMHInstanceToUser

type Encoding string

const (
	EncodingOld    Encoding = "old"
	EncodingNew    Encoding = "new"
	EncodingCorev1 Encoding = "corev1"
)

func ChooseEncoder(encoding Encoding) {
	switch encoding {
	case EncodingOld:
		encodeMessageFromUserToUMHInstance = encoding_old.EncodeMessageFromUserToUMHInstance
		encodeMessageFromUMHInstanceToUser = encoding_old.EncodeMessageFromUMHInstanceToUser
		decodeMessageFromUserToUMHInstance = encoding_old.DecodeMessageFromUserToUMHInstance
		decodeMessageFromUMHInstanceToUser = encoding_old.DecodeMessageFromUMHInstanceToUser
	case EncodingNew:
		encodeMessageFromUserToUMHInstance = encoding_new.EncodeMessageFromUserToUMHInstance
		encodeMessageFromUMHInstanceToUser = encoding_new.EncodeMessageFromUMHInstanceToUser
		decodeMessageFromUserToUMHInstance = encoding_new.DecodeMessageFromUserToUMHInstance
		decodeMessageFromUMHInstanceToUser = encoding_new.DecodeMessageFromUMHInstanceToUser
	case EncodingCorev1:
		encodeMessageFromUserToUMHInstance = encoding_corev1.EncodeMessageFromUserToUMHInstance
		encodeMessageFromUMHInstanceToUser = encoding_corev1.EncodeMessageFromUMHInstanceToUser
		decodeMessageFromUserToUMHInstance = encoding_corev1.DecodeMessageFromUserToUMHInstance
		decodeMessageFromUMHInstanceToUser = encoding_corev1.DecodeMessageFromUMHInstanceToUser
	}
}

func EncodeMessageFromUserToUMHInstance(UMHMessage models.UMHMessageContent) (string, error) {
	return encodeMessageFromUserToUMHInstance(UMHMessage)
}

func EncodeMessageFromUMHInstanceToUser(UMHMessage models.UMHMessageContent) (string, error) {
	return encodeMessageFromUMHInstanceToUser(UMHMessage)
}

func DecodeMessageFromUserToUMHInstance(base64Message string) (models.UMHMessageContent, error) {
	return decodeMessageFromUserToUMHInstance(base64Message)
}

func DecodeMessageFromUMHInstanceToUser(base64Message string) (models.UMHMessageContent, error) {
	return decodeMessageFromUMHInstanceToUser(base64Message)
}
