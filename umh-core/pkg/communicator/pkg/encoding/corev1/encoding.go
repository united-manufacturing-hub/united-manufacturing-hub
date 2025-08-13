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

/*
	fl@Ferdinands-MBP  ~/Git/umh3/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding   fix-subscriber-perf ✚  go test -bench=. -benchmem -count=1

Running Suite: Encoding Suite - /Users/fl/Git/umh3/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding
==================================================================================================================
Random Seed: 1751294110

Will run 50 of 50 specs
••••••••••••••••••••••••••••SSSSSSSSSSSSS•••••••••

Ran 37 of 50 Specs in 0.990 seconds
SUCCESS! -- 37 Passed | 0 Failed | 0 Pending | 13 Skipped
goos: darwin
goarch: arm64
pkg: github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding
cpu: Apple M3 Pro
BenchmarkCompress_Small/Old-11            160954             73261 ns/op         2345174 B/op         46 allocs/op
BenchmarkCompress_Small/New-11          145926442               90.87 ns/op          512 B/op          1 allocs/op
BenchmarkCompress_Small/CoreV1-11       1000000000               3.369 ns/op           0 B/op          0 allocs/op
BenchmarkCompress_Large/New-11           4330281              2801 ns/op            8544 B/op          3 allocs/op
BenchmarkCompress_Large/CoreV1-11        5960959              2032 ns/op             291 B/op          1 allocs/op
BenchmarkCompress_Large/Old-11            152952             78401 ns/op         2360534 B/op         46 allocs/op
BenchmarkEncodeMessage_Small/CoreV1-11          27924342               427.1 ns/op           451 B/op          5 allocs/op
BenchmarkEncodeMessage_Small/Old-11               146062             76747 ns/op         2329623 B/op         41 allocs/op
BenchmarkEncodeMessage_Small/New-11             27362049               439.8 ns/op           473 B/op          6 allocs/op
BenchmarkEncodeMessage_Medium/New-11             1000000             10902 ns/op            7517 B/op          9 allocs/op
BenchmarkEncodeMessage_Medium/CoreV1-11          1000000             10589 ns/op            4366 B/op          6 allocs/op
BenchmarkEncodeMessage_Medium/Old-11               98260            123455 ns/op         2357740 B/op         62 allocs/op
BenchmarkEncodeMessage_Large/New-11                 1120          10882589 ns/op        49284078 B/op        320 allocs/op
BenchmarkEncodeMessage_Large/CoreV1-11              1156          10292597 ns/op        41714245 B/op        527 allocs/op
BenchmarkEncodeMessage_Large/Old-11                 1005          11420631 ns/op        75887848 B/op        267 allocs/op
BenchmarkDecodeMessage_Small/CoreV1-11          19624303               601.4 ns/op           854 B/op         17 allocs/op
BenchmarkDecodeMessage_Small/Old-11              1441017              8481 ns/op            9603 B/op         57 allocs/op
BenchmarkDecodeMessage_Small/New-11             18378741               648.6 ns/op           874 B/op         18 allocs/op
BenchmarkDecodeMessage_Large/New-11                 1924           6330180 ns/op        29971921 B/op      60218 allocs/op
BenchmarkDecodeMessage_Large/CoreV1-11              1677           7078994 ns/op        29925609 B/op      60273 allocs/op
BenchmarkDecodeMessage_Large/Old-11                 1725           7067393 ns/op        46719564 B/op      60283 allocs/op
BenchmarkRoundTrip_Small/Old-11                   109585            106388 ns/op         2340724 B/op        103 allocs/op
BenchmarkRoundTrip_Small/New-11                 10648519              1172 ns/op            1352 B/op         24 allocs/op
BenchmarkRoundTrip_Small/CoreV1-11              11216079              1098 ns/op            1312 B/op         22 allocs/op
BenchmarkRoundTrip_Large/New-11                      716          16797878 ns/op        90145858 B/op      60612 allocs/op
BenchmarkRoundTrip_Large/CoreV1-11                   652          18386441 ns/op        73321625 B/op      60822 allocs/op
BenchmarkRoundTrip_Large/Old-11                      638          18625770 ns/op        128029471 B/op     60553 allocs/op
BenchmarkConcurrent/New-11                       2441404              5036 ns/op           26833 B/op        194 allocs/op
BenchmarkConcurrent/CoreV1-11                    2502412              4791 ns/op           23323 B/op        190 allocs/op
BenchmarkConcurrent/Old-11                        101326            125472 ns/op         2414367 B/op        295 allocs/op
BenchmarkMemoryIntensive/New-11                       13         826302747 ns/op        3090457942 B/op  3025548 allocs/op
BenchmarkMemoryIntensive/CoreV1-11                    13         878016208 ns/op        2857213023 B/op  3035773 allocs/op
BenchmarkMemoryIntensive/Old-11                       12         908707083 ns/op        5572278949 B/op  3029523 allocs/op
PASS
*/
package encoding_corev1

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

// Constants for buffer management.
const (
	CompressionThreshold = 1024       // 1KB
	MaxPooledBufferSize  = 256 * 1024 // 256KB max for all pools
	DefaultBufferSize    = 4 * 1024   // 4KB default
	Base64BufferSize     = 8 * 1024   // 8KB for base64 ops
	CompressBufferSize   = 16 * 1024  // 16KB for compression
	DecompressBufferSize = 32 * 1024  // 32KB for decompression
	JSONBufferSize       = 4 * 1024   // 4KB for JSON operations
)

var (
	// Encoder/decoder pools - optimized for speed.
	encoderPool = sync.Pool{
		New: func() interface{} {
			encoder, _ := zstd.NewWriter(nil,
				zstd.WithEncoderLevel(zstd.SpeedFastest),
				zstd.WithWindowSize(32*1024)) // Smaller window for faster encoding

			return encoder
		},
	}

	decoderPool = sync.Pool{
		New: func() interface{} {
			decoder, _ := zstd.NewReader(nil)

			return decoder
		},
	}

	// Buffer pools - store slices directly, not pointers.
	base64BufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, 0, Base64BufferSize)

			return &b
		},
	}

	compressBufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, CompressBufferSize))
		},
	}

	decompressBufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, DecompressBufferSize))
		},
	}
)

// Buffer management functions.
func getBase64Buffer() *[]byte {
	if buf, ok := base64BufferPool.Get().(*[]byte); ok {
		return buf
	}
	// If pool returns wrong type, create new buffer
	newBuf := make([]byte, 0, 1024)

	return &newBuf
}

func putBase64Buffer(b *[]byte) {
	if cap(*b) <= MaxPooledBufferSize {
		*b = (*b)[:0] // reset length
		base64BufferPool.Put(b)
	}
}

func getCompressBuffer() *bytes.Buffer {
	return compressBufferPool.Get().(*bytes.Buffer)
}

func putCompressBuffer(buf *bytes.Buffer) {
	if cap(buf.Bytes()) <= MaxPooledBufferSize {
		buf.Reset()
		compressBufferPool.Put(buf)
	}
}

func getDecompressBuffer() *bytes.Buffer {
	return decompressBufferPool.Get().(*bytes.Buffer)
}

func putDecompressBuffer(buf *bytes.Buffer) {
	if cap(buf.Bytes()) <= MaxPooledBufferSize {
		buf.Reset()
		decompressBufferPool.Put(buf)
	}
}

// Optimized magic number check using unsafe for single memory read.
func isCompressed(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// Use binary package for portability (safer than unsafe)
	return binary.LittleEndian.Uint32(data) == 0xFD2FB528
}

// Compress compresses message if above threshold
// Returns original slice for small messages (zero-copy).
func Compress(message []byte) ([]byte, error) {
	if len(message) < CompressionThreshold {
		return message, nil // Zero-copy for small messages
	}

	encoder := encoderPool.Get().(*zstd.Encoder)
	defer encoderPool.Put(encoder)

	buf := getCompressBuffer()
	defer putCompressBuffer(buf)

	buf.Grow(len(message) / 2) // Estimate compressed size
	encoder.Reset(buf)

	if _, err := encoder.Write(message); err != nil {
		return nil, err
	}

	err := encoder.Close()
	if err != nil {
		return nil, err
	}

	// Return copy of the compressed data
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())

	return result, nil
}

// CompressWithContext adds cancellation support for large messages.
func CompressWithContext(ctx context.Context, message []byte) ([]byte, error) {
	if len(message) < CompressionThreshold {
		return message, nil
	}

	// Check context before expensive operation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return Compress(message)
}

// Decompress decompresses message if compressed.
func Decompress(message []byte) ([]byte, error) {
	if !isCompressed(message) {
		// Return copy to maintain consistent behavior
		result := make([]byte, len(message))
		copy(result, message)

		return result, nil
	}

	decoder := decoderPool.Get().(*zstd.Decoder)
	defer decoderPool.Put(decoder)

	buf := getDecompressBuffer()
	defer putDecompressBuffer(buf)

	err := decoder.Reset(bytes.NewReader(message))
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(buf, decoder); err != nil {
		return nil, err
	}

	// Return copy of decompressed data
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())

	return result, nil
}

// Optimized base64 encoding using strings.Builder.
func encodeBase64(data []byte) string {
	encodedLen := base64.StdEncoding.EncodedLen(len(data))

	// Use strings.Builder for efficient string creation
	var builder strings.Builder
	builder.Grow(encodedLen)

	buf := getBase64Buffer()
	if cap(*buf) < encodedLen {
		*buf = make([]byte, encodedLen)
	} else {
		*buf = (*buf)[:encodedLen]
	}
	defer putBase64Buffer(buf)

	base64.StdEncoding.Encode(*buf, data)
	builder.Write(*buf)

	return builder.String()
}

// Optimized base64 decoding with buffer reuse.
func decodeBase64(data string) ([]byte, error) {
	decodedLen := base64.StdEncoding.DecodedLen(len(data))

	buf := getBase64Buffer()
	if cap(*buf) < decodedLen {
		*buf = make([]byte, decodedLen)
	} else {
		*buf = (*buf)[:decodedLen]
	}
	defer putBase64Buffer(buf)

	n, err := base64.StdEncoding.Decode(*buf, []byte(data))
	if err != nil {
		return nil, err
	}

	// Return exact size
	result := make([]byte, n)
	copy(result, (*buf)[:n])

	return result, nil
}

// Core encoding functions.
func EncodeMessageFromUserToUMHInstance(UMHMessage models.UMHMessageContent) (string, error) {
	messageBytes, err := safejson.Marshal(UMHMessage)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to marshal UMHMessage: %v (%+v)", err, UMHMessage)

		return "", err
	}

	return encodeBase64(messageBytes), nil
}

func EncodeMessageFromUMHInstanceToUser(UMHMessage models.UMHMessageContent) (string, error) {
	messageBytes, err := safejson.Marshal(UMHMessage)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to marshal UMHMessage: %v (%+v)", err, UMHMessage)

		return "", err
	}

	// Compress if above threshold
	if len(messageBytes) >= CompressionThreshold {
		compressed, err := Compress(messageBytes)
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to compress message: %v", err)

			return "", err
		}

		return encodeBase64(compressed), nil
	}

	return encodeBase64(messageBytes), nil
}

// Optimized decode with single function for both paths.
func decodeBase64AndUnmarshal(base64Message string) (models.UMHMessageContent, error) {
	var UMHMessage models.UMHMessageContent

	messageBytes, err := decodeBase64(base64Message)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to decode base64 message: %v", err)

		return UMHMessage, err
	}

	// Fast path for uncompressed data
	if len(messageBytes) < 4 || !isCompressed(messageBytes) {
		err = safejson.Unmarshal(messageBytes, &UMHMessage)
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to unmarshal UMHMessage: %v", err)
		}

		return UMHMessage, err
	}

	// Decompress and unmarshal
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

func DecodeMessageFromUserToUMHInstance(base64Message string) (models.UMHMessageContent, error) {
	return decodeBase64AndUnmarshal(base64Message)
}

func DecodeMessageFromUMHInstanceToUser(base64Message string) (models.UMHMessageContent, error) {
	return decodeBase64AndUnmarshal(base64Message)
}
