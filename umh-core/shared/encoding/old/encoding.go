package encoding_old

import (
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/fail"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/safejson"
)

func Compress(message string) (string, error) {
	var b strings.Builder
	err := c(strings.NewReader(message), &b)
	return b.String(), err
}

func c(in io.Reader, out io.Writer) error {
	enc, err := zstd.NewWriter(out)
	if err != nil {
		return err
	}
	_, err = io.Copy(enc, in)
	if err != nil {
		enc.Close()
		return err
	}
	return enc.Close()
}

func Decompress(message string) (string, error) {
	var b strings.Builder
	err := d(strings.NewReader(message), &b)
	return b.String(), err
}

func d(in io.Reader, out io.Writer) error {
	dec, err := zstd.NewReader(in)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, dec)
	if err != nil {
		dec.Close()
		return err
	}
	dec.Close()
	return nil
}

// EncodeMessageFromUserToUMHInstance converts and encodes a UMHMessageContent object to Base64 String.
// Note: only the inner payload will later be encrypted, not the whole message.
func EncodeMessageFromUserToUMHInstance(UMHMessage models.UMHMessageContent) (string, error) {
	messageBytes, err := safejson.Marshal(UMHMessage)
	if err != nil {
		fail.ErrorBatchedf("Failed to marshal UMHMessage: %v (%+v)", err, UMHMessage)
		//		zap.S().Debugf("Payload Type: %T", UMHMessage.Payload)
		return "", err
	}
	return base64.StdEncoding.EncodeToString(messageBytes), nil
}

// EncodeMessageFromUMHInstanceToUser converts and encodes a UMHMessageContent object to Base64 String.
// Note: only the inner payload will later be encrypted, not the whole message.
func EncodeMessageFromUMHInstanceToUser(UMHMessage models.UMHMessageContent) (string, error) {
	messageBytes, err := safejson.Marshal(UMHMessage)
	if err != nil {
		fail.ErrorBatchedf("Failed to marshal UMHMessage: %v (%+v)", err, UMHMessage)
		//		zap.S().Debugf("Payload Type: %T", UMHMessage.Payload)
		return "", err
	}
	compressed, err := Compress(string(messageBytes))
	if err != nil {
		fail.ErrorBatchedf("Failed to compress message: %v", err)
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(compressed))
	return encoded, nil
}

// DecodeMessageFromUserToUMHInstance decodes a Base64 String to a UMHMessageContent object.
// Note: only the inner payload will later be encrypted, not the whole message.
func DecodeMessageFromUserToUMHInstance(base64Message string) (UMHMessage models.UMHMessageContent, err error) {
	// Decode Base64 to JSON bytes
	messageBytes, err := base64.StdEncoding.DecodeString(base64Message)
	if err != nil {
		fail.ErrorBatchedf("Failed to decode base64 message: %v", err)
		return
	}

	hexEncoded := hex.EncodeToString(messageBytes)

	// Check magic (28 b5 2f fd)

	// User messages shouldn't be compressed (yet), but we check it anyway
	if strings.HasPrefix(hexEncoded, "28b52ffd") {
		// Decompress
		var decompressedMessage string
		decompressedMessage, err = Decompress(string(messageBytes))
		if err != nil {
			fail.ErrorBatchedf("Failed to decompress base64 message: %v", err)
			return
		}
		err = safejson.Unmarshal([]byte(decompressedMessage), &UMHMessage)
		if err != nil {
			fail.ErrorBatchedf("Failed to unmarshal UMHMessage: %v", err)
			return
		}
	} else {
		err = safejson.Unmarshal(messageBytes, &UMHMessage)
		if err != nil {
			fail.ErrorBatchedf("Failed to unmarshal UMHMessage: %v", err)
			return
		}
	}
	return
}

// DecodeMessageFromUMHInstanceToUser decodes a Base64 String to a UMHMessageContent object.
// Note: only the inner payload will later be encrypted, not the whole message.
func DecodeMessageFromUMHInstanceToUser(base64Message string) (UMHMessage models.UMHMessageContent, err error) {
	// Decode Base64 to JSON bytes
	messageBytes, err := base64.StdEncoding.DecodeString(base64Message)
	if err != nil {
		fail.ErrorBatchedf("Failed to decode base64 message: %v", err)
		return
	}

	hexEncoded := hex.EncodeToString(messageBytes)

	// Check magic (28 b5 2f fd)

	if strings.HasPrefix(hexEncoded, "28b52ffd") {
		// Decompress
		var decompressedMessage string
		decompressedMessage, err = Decompress(string(messageBytes))
		if err != nil {
			fail.ErrorBatchedf("Failed to decompress base64 message: %v", err)
			return
		}
		err = safejson.Unmarshal([]byte(decompressedMessage), &UMHMessage)
		if err != nil {
			fail.ErrorBatchedf("Failed to unmarshal UMHMessage: %v", err)
			return
		}
	} else {
		err = safejson.Unmarshal(messageBytes, &UMHMessage)
		if err != nil {
			fail.ErrorBatchedf("Failed to unmarshal UMHMessage: %v", err)
			return
		}
	}

	return
}
