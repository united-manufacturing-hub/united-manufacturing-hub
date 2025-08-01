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

package encoding_new

import (
	"bytes"
	"encoding/base64"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

// CompressionThreshold is the size in bytes above which messages will be compressed
const CompressionThreshold = 1024 // 1KB

// Mutex for the encoder/decoder pools
var encoderMutex sync.Mutex
var decoderMutex sync.Mutex

var (
	// Global encoder/decoder pools
	encoderPool = sync.Pool{
		New: func() interface{} {
			encoder, _ := zstd.NewWriter(nil,
				zstd.WithEncoderLevel(zstd.SpeedFastest)) // Optimize for speed
			return encoder
		},
	}

	decoderPool = sync.Pool{
		New: func() interface{} {
			decoder, _ := zstd.NewReader(nil)
			return decoder
		},
	}

	// Add a new buffer pool for base64 operations
	base64BufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 0, 1024) // Pre-allocate with reasonable size
			return &buf
		},
	}

	// Add a buffer pool for decompression
	decompressBufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 32*1024)) // Pre-allocate 32KB
		},
	}
)

func getBase64Buffer() []byte {
	bufPtr := base64BufferPool.Get().(*[]byte)
	return *bufPtr
}

func putBase64Buffer(buf []byte) {
	if cap(buf) <= 32*1024 { // Only reuse reasonably sized buffers
		// Store a pointer to the slice to avoid allocations
		b := &buf
		*b = (*b)[:0]
		base64BufferPool.Put(b)
	}
}

func getDecompressBuffer() *bytes.Buffer {
	return decompressBufferPool.Get().(*bytes.Buffer)
}

func putDecompressBuffer(buf *bytes.Buffer) {
	if cap(buf.Bytes()) <= 1024*1024 { // Only reuse buffers up to 1MB
		buf.Reset()
		decompressBufferPool.Put(buf)
	}
}

func Compress(message []byte) ([]byte, error) {
	// Skip compression for small messages
	if len(message) < CompressionThreshold {
		// Create a copy to avoid data races
		result := make([]byte, len(message))
		copy(result, message)
		return result, nil
	}

	encoder := encoderPool.Get().(*zstd.Encoder)
	defer encoderPool.Put(encoder)

	// Create a new buffer for each compression
	b := new(bytes.Buffer)
	b.Grow(len(message)) // Pre-allocate with input size

	encoder.Reset(b)

	if _, err := encoder.Write(message); err != nil {
		return nil, err
	}

	if err := encoder.Close(); err != nil {
		return nil, err
	}

	// Return a copy of the bytes to avoid data races
	result := make([]byte, b.Len())
	copy(result, b.Bytes())
	return result, nil
}

func Decompress(message []byte) ([]byte, error) {
	// Skip decompression if not compressed
	if !isCompressed(message) {
		result := make([]byte, len(message))
		copy(result, message)
		return result, nil
	}

	decoder := decoderPool.Get().(*zstd.Decoder)
	defer decoderPool.Put(decoder)

	// Get buffer from pool
	b := getDecompressBuffer()
	defer putDecompressBuffer(b)

	err := decoder.Reset(bytes.NewReader(message))
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(b, decoder); err != nil {
		return nil, err
	}

	// Return a copy of the bytes to avoid data races
	result := make([]byte, b.Len())
	copy(result, b.Bytes())
	return result, nil
}

// isCompressed checks for zstd magic bytes (0x28 0xB5 0x2F 0xFD)
func isCompressed(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// Use uint32 comparison instead of byte-by-byte
	magic := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
	return magic == 0xFD2FB528
}

// EncodeMessageFromUserToUMHInstance converts and encodes a UMHMessageContent object to Base64 String.
// Note: only the inner payload will later be encrypted, not the whole message.
func EncodeMessageFromUserToUMHInstance(UMHMessage models.UMHMessageContent) (string, error) {
	encoderMutex.Lock()
	defer encoderMutex.Unlock()

	messageBytes, err := safejson.Marshal(UMHMessage)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to marshal UMHMessage: %v (%+v)", err, UMHMessage)
		return "", err
	}
	return encodeBase64(messageBytes), nil
}

// EncodeMessageFromUMHInstanceToUser converts and encodes a UMHMessageContent object to Base64 String.
// Note: only the inner payload will later be encrypted, not the whole message.
func EncodeMessageFromUMHInstanceToUser(UMHMessage models.UMHMessageContent) (string, error) {
	encoderMutex.Lock()
	defer encoderMutex.Unlock()

	messageBytes, err := safejson.Marshal(UMHMessage)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to marshal UMHMessage: %v (%+v)", err, UMHMessage)
		return "", err
	}

	// Only compress if message is large enough
	if len(messageBytes) >= CompressionThreshold {
		compressed, err := Compress(messageBytes)
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to compress message: %v", err)
			return "", err
		}
		return encodeBase64(compressed), nil
	}

	// Skip compression for small messages
	return encodeBase64(messageBytes), nil
}

// Helper function for base64 encoding
func encodeBase64(data []byte) string {
	// Calculate the exact size needed for base64 encoding
	encodedLen := base64.StdEncoding.EncodedLen(len(data))

	// Get a buffer from the pool
	buf := getBase64Buffer()
	if cap(buf) < encodedLen {
		buf = make([]byte, encodedLen)
	} else {
		buf = buf[:encodedLen]
	}
	defer putBase64Buffer(buf)

	// Encode directly into the buffer
	base64.StdEncoding.Encode(buf, data)
	return string(buf)
}

// Helper function for base64 decoding
func decodeBase64(data string) ([]byte, error) {
	// Calculate the maximum size needed for base64 decoding
	decodedLen := base64.StdEncoding.DecodedLen(len(data))

	// Get a buffer from the pool
	buf := getBase64Buffer()
	if cap(buf) < decodedLen {
		buf = make([]byte, decodedLen)
	} else {
		buf = buf[:decodedLen]
	}
	defer putBase64Buffer(buf)

	n, err := base64.StdEncoding.Decode(buf, []byte(data))
	if err != nil {
		return nil, err
	}

	// Return a copy of the exact size needed
	result := make([]byte, n)
	copy(result, buf[:n])
	return result, nil
}

// decodeBase64AndUnmarshal handles the common decoding logic
func decodeBase64AndUnmarshal(base64Message string) (models.UMHMessageContent, error) {
	var UMHMessage models.UMHMessageContent

	// Get buffer from pool for base64 decoding
	messageBytes, err := decodeBase64(base64Message)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to decode base64 message: %v", err)
		return UMHMessage, err
	}

	// Fast path: if not compressed, unmarshal directly
	// If the message is shorter then 4 bytes, it cannot be compressed, as the magic bytes are at least 4 bytes long
	if len(messageBytes) < 4 || !isCompressed(messageBytes) {
		err = safejson.Unmarshal(messageBytes, &UMHMessage)
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to unmarshal UMHMessage: %v", err)
		}
		return UMHMessage, err
	}

	// Compressed path
	decompressedMessage, err := Decompress(messageBytes)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to decompress message: %v", err)
		return UMHMessage, err
	}

	err = safejson.Unmarshal(decompressedMessage, &UMHMessage)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to unmarshal UMHMessage: %v", err)
	}
	return UMHMessage, err
}

// DecodeMessageFromUserToUMHInstance decodes a Base64 String to a UMHMessageContent object.
func DecodeMessageFromUserToUMHInstance(base64Message string) (models.UMHMessageContent, error) {
	decoderMutex.Lock()
	defer decoderMutex.Unlock()

	return decodeBase64AndUnmarshal(base64Message)
}

// DecodeMessageFromUMHInstanceToUser decodes a Base64 String to a UMHMessageContent object.
func DecodeMessageFromUMHInstanceToUser(base64Message string) (models.UMHMessageContent, error) {
	decoderMutex.Lock()
	defer decoderMutex.Unlock()

	return decodeBase64AndUnmarshal(base64Message)
}
