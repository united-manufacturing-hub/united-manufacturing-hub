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
	"fmt"
	"strings"
	"testing"

	encoding_corev1 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding/corev1"
	encoding_new "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding/new"
	encoding_old "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding/old"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// EncodingImpl holds the functions for a specific encoding implementation.
type EncodingImpl struct {
	Name                               string
	EncodeMessageFromUMHInstanceToUser func(models.UMHMessageContent) ([]byte, error)
	DecodeMessageFromUMHInstanceToUser func([]byte) (models.UMHMessageContent, error)
	Compress                           func([]byte) ([]byte, error)
}

// Wrapper functions to normalize different implementations.
func wrapNewEncode(msg models.UMHMessageContent) ([]byte, error) {
	result, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)

	return []byte(result), err
}

func wrapNewDecode(data []byte) (models.UMHMessageContent, error) {
	return encoding_new.DecodeMessageFromUMHInstanceToUser(string(data))
}

func wrapCorev1Encode(msg models.UMHMessageContent) ([]byte, error) {
	result, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)

	return []byte(result), err
}

func wrapCorev1Decode(data []byte) (models.UMHMessageContent, error) {
	return encoding_corev1.DecodeMessageFromUMHInstanceToUser(string(data))
}

func wrapOldEncode(msg models.UMHMessageContent) ([]byte, error) {
	result, err := encoding_old.EncodeMessageFromUMHInstanceToUser(msg)

	return []byte(result), err
}

func wrapOldDecode(data []byte) (models.UMHMessageContent, error) {
	return encoding_old.DecodeMessageFromUMHInstanceToUser(string(data))
}

func wrapNewCompress(data []byte) ([]byte, error) {
	return encoding_new.Compress(data)
}

func wrapCorev1Compress(data []byte) ([]byte, error) {
	return encoding_corev1.Compress(data)
}

func wrapOldCompress(data []byte) ([]byte, error) {
	compressed, err := encoding_old.Compress(string(data))
	if err != nil {
		return nil, err
	}

	return []byte(compressed), nil
}

// encodingImplementations holds all the encoding implementations to benchmark.
var encodingImplementations = map[string]EncodingImpl{
	"New": {
		Name:                               "New",
		EncodeMessageFromUMHInstanceToUser: wrapNewEncode,
		DecodeMessageFromUMHInstanceToUser: wrapNewDecode,
		Compress:                           wrapNewCompress,
	},
	"CoreV1": {
		Name:                               "CoreV1",
		EncodeMessageFromUMHInstanceToUser: wrapCorev1Encode,
		DecodeMessageFromUMHInstanceToUser: wrapCorev1Decode,
		Compress:                           wrapCorev1Compress,
	},
	"Old": {
		Name:                               "Old",
		EncodeMessageFromUMHInstanceToUser: wrapOldEncode,
		DecodeMessageFromUMHInstanceToUser: wrapOldDecode,
		Compress:                           wrapOldCompress,
	},
}

// Test data generators.
func generateSmallMessage() models.UMHMessageContent {
	return models.UMHMessageContent{
		MessageType: "test",
		Payload: map[string]interface{}{
			"value":     42,
			"timestamp": 1234567890,
			"status":    "ok",
		},
	}
}

func generateMediumMessage() models.UMHMessageContent {
	data := make(map[string]interface{})
	for i := range 50 {
		data[fmt.Sprintf("field_%d", i)] = fmt.Sprintf("value_%d_%s", i, strings.Repeat("x", 20))
	}

	return models.UMHMessageContent{
		MessageType: "medium_test",
		Payload:     data,
	}
}

func generateLargeMessage() models.UMHMessageContent {
	data := make(map[string]interface{})
	for i := range 20000 {
		data[fmt.Sprintf("field_%d", i)] = fmt.Sprintf("value_%d_%s", i, strings.Repeat("data", 50))
	}

	return models.UMHMessageContent{
		MessageType: "large_test",
		Payload:     data,
	}
}

func generateBinaryData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	return data
}

// Generic benchmark functions.
func benchmarkCompress(b *testing.B, impl EncodingImpl, size int) {
	b.Helper()
	b.Run(impl.Name, func(b *testing.B) {
		data := generateBinaryData(size)

		b.ResetTimer()
		b.ReportAllocs()

		for range b.N {
			_, err := impl.Compress(data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCompress_Small(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkCompress(b, impl, 512)
	}
}

func BenchmarkCompress_Large(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkCompress(b, impl, 8192)
	}
}

func benchmarkEncodeMessage(b *testing.B, impl EncodingImpl, msgGenerator func() models.UMHMessageContent) {
	b.Helper()
	b.Run(impl.Name, func(b *testing.B) {
		msg := msgGenerator()

		b.ResetTimer()
		b.ReportAllocs()

		for range b.N {
			_, err := impl.EncodeMessageFromUMHInstanceToUser(msg)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkEncodeMessage_Small(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkEncodeMessage(b, impl, generateSmallMessage)
	}
}

func BenchmarkEncodeMessage_Medium(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkEncodeMessage(b, impl, generateMediumMessage)
	}
}

func BenchmarkEncodeMessage_Large(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkEncodeMessage(b, impl, generateLargeMessage)
	}
}

func benchmarkDecodeMessage(b *testing.B, impl EncodingImpl, msgGenerator func() models.UMHMessageContent) {
	b.Helper()
	b.Run(impl.Name, func(b *testing.B) {
		msg := msgGenerator()

		encoded, err := impl.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for range b.N {
			_, err := impl.DecodeMessageFromUMHInstanceToUser(encoded)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkDecodeMessage_Small(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkDecodeMessage(b, impl, generateSmallMessage)
	}
}

func BenchmarkDecodeMessage_Large(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkDecodeMessage(b, impl, generateLargeMessage)
	}
}

func benchmarkRoundTrip(b *testing.B, impl EncodingImpl, msgGenerator func() models.UMHMessageContent) {
	b.Helper()

	b.Run(impl.Name, func(b *testing.B) {
		msg := msgGenerator()

		b.ResetTimer()
		b.ReportAllocs()

		for range b.N {
			encoded, err := impl.EncodeMessageFromUMHInstanceToUser(msg)
			if err != nil {
				b.Fatal(err)
			}

			_, err = impl.DecodeMessageFromUMHInstanceToUser(encoded)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkRoundTrip_Small(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkRoundTrip(b, impl, generateSmallMessage)
	}
}

func BenchmarkRoundTrip_Large(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkRoundTrip(b, impl, generateLargeMessage)
	}
}

func benchmarkConcurrent(b *testing.B, impl EncodingImpl) {
	b.Helper()

	b.Run(impl.Name, func(b *testing.B) {
		msg := generateMediumMessage()

		b.ResetTimer()
		b.ReportAllocs()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				encoded, err := impl.EncodeMessageFromUMHInstanceToUser(msg)
				if err != nil {
					b.Fatal(err)
				}

				_, err = impl.DecodeMessageFromUMHInstanceToUser(encoded)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

func BenchmarkConcurrent(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkConcurrent(b, impl)
	}
}

func benchmarkMemoryIntensive(b *testing.B, impl EncodingImpl) {
	b.Helper()

	b.Run(impl.Name, func(b *testing.B) {
		messages := make([]models.UMHMessageContent, 100)
		for i := range messages {
			if i%2 == 0 {
				messages[i] = generateSmallMessage()
			} else {
				messages[i] = generateLargeMessage()
			}
		}

		b.ResetTimer()
		b.ReportAllocs()

		for range b.N {
			for _, msg := range messages {
				encoded, err := impl.EncodeMessageFromUMHInstanceToUser(msg)
				if err != nil {
					b.Fatal(err)
				}

				_, err = impl.DecodeMessageFromUMHInstanceToUser(encoded)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

func BenchmarkMemoryIntensive(b *testing.B) {
	for _, impl := range encodingImplementations {
		benchmarkMemoryIntensive(b, impl)
	}
}
