package encoding

import (
	encoding_new "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding/new"
	encoding_old "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding/old"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
	"go.uber.org/zap"
)

var encodeMessageFromUserToUMHInstance func(UMHMessage models.UMHMessageContent) (string, error) = encoding_old.EncodeMessageFromUserToUMHInstance
var encodeMessageFromUMHInstanceToUser func(UMHMessage models.UMHMessageContent) (string, error) = encoding_old.EncodeMessageFromUMHInstanceToUser
var decodeMessageFromUserToUMHInstance func(base64Message string) (models.UMHMessageContent, error) = encoding_old.DecodeMessageFromUserToUMHInstance
var decodeMessageFromUMHInstanceToUser func(base64Message string) (models.UMHMessageContent, error) = encoding_old.DecodeMessageFromUMHInstanceToUser

func EnableNewEncoder() {
	zap.S().Info("Enabling new encoder")
	encodeMessageFromUserToUMHInstance = encoding_new.EncodeMessageFromUserToUMHInstance
	encodeMessageFromUMHInstanceToUser = encoding_new.EncodeMessageFromUMHInstanceToUser
	decodeMessageFromUserToUMHInstance = encoding_new.DecodeMessageFromUserToUMHInstance
	decodeMessageFromUMHInstanceToUser = encoding_new.DecodeMessageFromUMHInstanceToUser
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
