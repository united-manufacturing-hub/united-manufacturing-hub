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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// Test data generators
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
	for i := 0; i < 50; i++ {
		data[fmt.Sprintf("field_%d", i)] = fmt.Sprintf("value_%d_%s", i, strings.Repeat("x", 20))
	}

	return models.UMHMessageContent{
		MessageType: "medium_test",
		Payload:     data,
	}
}

func generateLargeMessage() models.UMHMessageContent {
	data := make(map[string]interface{})
	for i := 0; i < 20000; i++ {
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

// Compression benchmarks
func BenchmarkCompress_Small_Old(b *testing.B) {
	data := generateBinaryData(512) // Below threshold
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_new.Compress(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompress_Small_CoreV1(b *testing.B) {
	data := generateBinaryData(512) // Below threshold
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_corev1.Compress(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompress_Large_Old(b *testing.B) {
	data := generateBinaryData(8192) // Above threshold
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_new.Compress(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompress_Large_CoreV1(b *testing.B) {
	data := generateBinaryData(8192) // Above threshold
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_corev1.Compress(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Message encoding benchmarks
func BenchmarkEncodeMessage_Small_Old(b *testing.B) {
	msg := generateSmallMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMessage_Small_CoreV1(b *testing.B) {
	msg := generateSmallMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMessage_Medium_Old(b *testing.B) {
	msg := generateMediumMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMessage_Medium_CoreV1(b *testing.B) {
	msg := generateMediumMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMessage_Large_Old(b *testing.B) {
	msg := generateLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMessage_Large_CoreV1(b *testing.B) {
	msg := generateLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Decode benchmarks
func BenchmarkDecodeMessage_Small_Old(b *testing.B) {
	msg := generateSmallMessage()
	encoded, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_new.DecodeMessageFromUMHInstanceToUser(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeMessage_Small_CoreV1(b *testing.B) {
	msg := generateSmallMessage()
	encoded, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_corev1.DecodeMessageFromUMHInstanceToUser(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeMessage_Large_Old(b *testing.B) {
	msg := generateLargeMessage()
	encoded, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_new.DecodeMessageFromUMHInstanceToUser(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeMessage_Large_CoreV1(b *testing.B) {
	msg := generateLargeMessage()
	encoded, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_corev1.DecodeMessageFromUMHInstanceToUser(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Round-trip benchmarks (encode + decode)
func BenchmarkRoundTrip_Small_Old(b *testing.B) {
	msg := generateSmallMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		encoded, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
		_, err = encoding_new.DecodeMessageFromUMHInstanceToUser(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoundTrip_Small_CoreV1(b *testing.B) {
	msg := generateSmallMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		encoded, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
		_, err = encoding_corev1.DecodeMessageFromUMHInstanceToUser(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoundTrip_Large_Old(b *testing.B) {
	msg := generateLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		encoded, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
		_, err = encoding_new.DecodeMessageFromUMHInstanceToUser(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoundTrip_Large_CoreV1(b *testing.B) {
	msg := generateLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		encoded, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)
		if err != nil {
			b.Fatal(err)
		}
		_, err = encoding_corev1.DecodeMessageFromUMHInstanceToUser(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Batch processing benchmarks (CoreV1 specific feature)
func BenchmarkBatchEncode_CoreV1_Small(b *testing.B) {
	messages := make([]models.UMHMessageContent, 10)
	for i := range messages {
		messages[i] = generateSmallMessage()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_corev1.EncodeBatchMessages(messages)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBatchEncode_CoreV1_Large(b *testing.B) {
	messages := make([]models.UMHMessageContent, 10)
	for i := range messages {
		messages[i] = generateLargeMessage()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := encoding_corev1.EncodeBatchMessages(messages)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Concurrent benchmarks
func BenchmarkConcurrent_Old(b *testing.B) {
	msg := generateMediumMessage()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			encoded, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)
			if err != nil {
				b.Fatal(err)
			}
			_, err = encoding_new.DecodeMessageFromUMHInstanceToUser(encoded)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkConcurrent_CoreV1(b *testing.B) {
	msg := generateMediumMessage()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			encoded, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)
			if err != nil {
				b.Fatal(err)
			}
			_, err = encoding_corev1.DecodeMessageFromUMHInstanceToUser(encoded)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Memory usage benchmarks with different scenarios
func BenchmarkMemoryIntensive_Old(b *testing.B) {
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

	for i := 0; i < b.N; i++ {
		for _, msg := range messages {
			encoded, err := encoding_new.EncodeMessageFromUMHInstanceToUser(msg)
			if err != nil {
				b.Fatal(err)
			}
			_, err = encoding_new.DecodeMessageFromUMHInstanceToUser(encoded)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkMemoryIntensive_CoreV1(b *testing.B) {
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

	for i := 0; i < b.N; i++ {
		for _, msg := range messages {
			encoded, err := encoding_corev1.EncodeMessageFromUMHInstanceToUser(msg)
			if err != nil {
				b.Fatal(err)
			}
			_, err = encoding_corev1.DecodeMessageFromUMHInstanceToUser(encoded)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
