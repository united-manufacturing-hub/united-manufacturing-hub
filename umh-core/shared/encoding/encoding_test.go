package encoding_test

import (
	"encoding/base64"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	new "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/encoding/new"
	old "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/encoding/old"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/tools/safejson"
)

var _ = Describe("Encode", func() {
	Context("encode", func() {
		var (
			messageContent models.UMHMessageContent
		)

		BeforeEach(func() {
			messageContent = models.UMHMessageContent{
				MessageType: models.Status,
				Payload:     `{"hello": "world"}`,
			}
		})

		It("should encode message from user to UMH instance", func() {
			encodedMessage, err := new.EncodeMessageFromUserToUMHInstance(messageContent)

			Expect(err).To(BeNil())
			Expect(encodedMessage).ToNot(BeEmpty())
		})

		It("should encode message from UMH instance to user", func() {
			encodedMessage, err := new.EncodeMessageFromUMHInstanceToUser(messageContent)

			Expect(err).To(BeNil())
			Expect(encodedMessage).ToNot(BeEmpty())
		})
	})

	Context("decode", func() {
		var (
			messageContent models.UMHMessageContent
			encodedMessage string
		)

		BeforeEach(func() {
			messageContent = models.UMHMessageContent{
				MessageType: models.Status,
				Payload:     `{"hello": "world"}`,
			}
			var err error
			encodedMessage, err = new.EncodeMessageFromUserToUMHInstance(messageContent)
			Expect(err).To(BeNil())
		})

		It("should decode message from user to UMH instance", func() {
			decodedMessage, err := new.DecodeMessageFromUserToUMHInstance(encodedMessage)

			Expect(err).To(BeNil())
			Expect(decodedMessage).To(Equal(messageContent))
		})

		It("should decode message from UMH instance to user", func() {
			decodedMessage, err := new.DecodeMessageFromUMHInstanceToUser(encodedMessage)

			Expect(err).To(BeNil())
			Expect(decodedMessage).To(Equal(messageContent))
		})

		It("should decode an compressed message from UMH Instance to User", func() {
			encodedMessage, err := new.EncodeMessageFromUMHInstanceToUser(messageContent)
			Expect(err).To(BeNil())

			decodedMessage, err := new.DecodeMessageFromUMHInstanceToUser(encodedMessage)
			Expect(err).To(BeNil())
			Expect(decodedMessage).To(Equal(messageContent))
		})
	})
})

var _ = Describe("ZSTD", func() {
	It("compresses and decompresses data", func() {
		data := models.UMHMessageContent{
			Payload:     "hello worldhello worldhello world",
			MessageType: models.ActionReply,
		}
		xData, err := safejson.Marshal(data)
		Expect(err).To(BeNil())

		compressedData, err := new.Compress(xData)
		Expect(err).To(BeNil())

		// Base64 encode the string to be able to test against frontend
		encodedData := base64.StdEncoding.EncodeToString([]byte(compressedData))
		GinkgoWriter.Write([]byte(encodedData))

		decompressedData, err := new.Decompress(compressedData)
		Expect(err).To(BeNil())

		var data2 models.UMHMessageContent
		err = safejson.Unmarshal([]byte(decompressedData), &data2)
		Expect(err).To(BeNil())
	})
})

var _ = Describe("Compatibility", func() {
	var (
		smallMessage models.UMHMessageContent
		largeMessage models.UMHMessageContent
	)

	BeforeEach(func() {
		smallMessage = models.UMHMessageContent{
			MessageType: models.Status,
			Payload:     `{"hello": "world"}`,
		}

		// Create a larger payload
		largePayload := make([]string, 100)
		for i := range largePayload {
			largePayload[i] = fmt.Sprintf(`{"key_%d": "value_%d_with_some_padding"}`, i, i)
		}
		largeMessage = models.UMHMessageContent{
			MessageType: models.Status,
			Payload:     fmt.Sprintf(`{"items": [%s]}`, safejson.MustMarshal(largePayload)),
		}
	})

	Context("Backwards Compatibility", func() {
		It("should decode old-encoded small messages with new decoder", func() {
			oldEncoded, err := old.EncodeMessageFromUMHInstanceToUser(smallMessage)
			Expect(err).NotTo(HaveOccurred())

			decoded, err := new.DecodeMessageFromUMHInstanceToUser(oldEncoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded).To(Equal(smallMessage))
		})

		It("should decode old-encoded large messages with new decoder", func() {
			oldEncoded, err := old.EncodeMessageFromUMHInstanceToUser(largeMessage)
			Expect(err).NotTo(HaveOccurred())

			decoded, err := new.DecodeMessageFromUMHInstanceToUser(oldEncoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded).To(Equal(largeMessage))
		})

		It("should decode new-encoded messages with old decoder", func() {
			newEncoded, err := new.EncodeMessageFromUMHInstanceToUser(largeMessage)
			Expect(err).NotTo(HaveOccurred())

			decoded, err := old.DecodeMessageFromUMHInstanceToUser(newEncoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded).To(Equal(largeMessage))
		})

		It("should handle compression threshold differences correctly", func() {
			// Encode a message just below and just above the threshold
			mediumMessage := models.UMHMessageContent{
				MessageType: models.Status,
				Payload:     strings.Repeat("x", new.CompressionThreshold-100), // Just below threshold
			}

			newEncoded, err := new.EncodeMessageFromUMHInstanceToUser(mediumMessage)
			Expect(err).NotTo(HaveOccurred())

			decoded, err := old.DecodeMessageFromUMHInstanceToUser(newEncoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded).To(Equal(mediumMessage))

			// Now with a message just above threshold
			mediumMessage.Payload = strings.Repeat("x", new.CompressionThreshold+100)
			newEncoded, err = new.EncodeMessageFromUMHInstanceToUser(mediumMessage)
			Expect(err).NotTo(HaveOccurred())

			decoded, err = old.DecodeMessageFromUMHInstanceToUser(newEncoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded).To(Equal(mediumMessage))
		})
	})
})

var _ = Describe("Performance Comparison", Serial, Label("measurement"), func() {
	BeforeEach(func() {
		Skip("Skipping performance tests due to unreliable test runners")
	})

	var (
		smallMessage models.UMHMessageContent
		largeMessage models.UMHMessageContent
		experiment   *gmeasure.Experiment
	)

	BeforeEach(func() {
		smallMessage = models.UMHMessageContent{
			MessageType: models.Status,
			Payload:     `{"hello": "world"}`,
		}

		// Create a larger payload that will trigger compression
		largePayload := make([]string, 100)
		for i := range largePayload {
			largePayload[i] = fmt.Sprintf(`{"key_%d": "value_%d_with_some_padding_to_make_it_longer"}`, i, i)
		}
		largeMessage = models.UMHMessageContent{
			MessageType: models.Status,
			Payload:     fmt.Sprintf(`{"items": [%s]}`, safejson.MustMarshal(largePayload)),
		}

		experiment = gmeasure.NewExperiment("Encoding Performance")
		AddReportEntry(experiment.Name, experiment)

		// Add warmup phase
		By("Warming up encoders and decoders")
		for i := 0; i < 1000; i++ {
			// Warm up with both small and large messages
			_, _ = new.EncodeMessageFromUMHInstanceToUser(smallMessage)
			_, _ = new.EncodeMessageFromUMHInstanceToUser(largeMessage)
			_, _ = old.EncodeMessageFromUMHInstanceToUser(smallMessage)
			_, _ = old.EncodeMessageFromUMHInstanceToUser(largeMessage)
		}
		runtime.GC() // Clean up after warmup
	})

	Context("Encoding Performance", func() {
		It("measures encoding performance improvements", func() {
			// Measure large message encoding
			experiment.Sample(func(idx int) {
				runtime.GC() // Ensure clean state before each sample
				experiment.MeasureDuration("new-large-encode", func() {
					_, err := new.EncodeMessageFromUMHInstanceToUser(largeMessage)
					Expect(err).NotTo(HaveOccurred())
				})
			}, gmeasure.SamplingConfig{N: 1000, Duration: time.Second * 10})

			experiment.Sample(func(idx int) {
				runtime.GC() // Ensure clean state before each sample
				experiment.MeasureDuration("old-large-encode", func() {
					_, err := old.EncodeMessageFromUMHInstanceToUser(largeMessage)
					Expect(err).NotTo(HaveOccurred())
				})
			}, gmeasure.SamplingConfig{N: 1000, Duration: time.Second * 10})

			// Compare and verify improvements
			newStats := experiment.GetStats("new-large-encode")
			oldStats := experiment.GetStats("old-large-encode")

			medianNew := newStats.DurationFor(gmeasure.StatMedian)
			medianOld := oldStats.DurationFor(gmeasure.StatMedian)

			Expect(medianNew).To(BeNumerically("<", medianOld))

			improvement := float64(medianOld-medianNew) / float64(medianOld) * 100
			experiment.RecordValue("Speed Improvement %", improvement)

			// Add detailed statistics
			By(fmt.Sprintf("Performance Statistics:"))
			By(fmt.Sprintf("New Implementation:"))
			By(fmt.Sprintf("  Median: %v", medianNew))
			By(fmt.Sprintf("  Mean: %v", newStats.DurationFor(gmeasure.StatMean)))
			By(fmt.Sprintf("  StdDev: %v", newStats.DurationFor(gmeasure.StatStdDev)))
			By(fmt.Sprintf("Old Implementation:"))
			By(fmt.Sprintf("  Median: %v", medianOld))
			By(fmt.Sprintf("  Mean: %v", oldStats.DurationFor(gmeasure.StatMean)))
			By(fmt.Sprintf("  StdDev: %v", oldStats.DurationFor(gmeasure.StatStdDev)))
			By(fmt.Sprintf("Improvement: %.2f%%", improvement))
		})

		It("measures memory allocations", func() {
			runtime.GC()
			var m1, m2 runtime.MemStats

			// Additional warmup specific to allocation testing
			for i := 0; i < 100; i++ {
				_, _ = new.EncodeMessageFromUMHInstanceToUser(largeMessage)
				_, _ = old.EncodeMessageFromUMHInstanceToUser(largeMessage)
			}
			runtime.GC()

			// Measure new implementation
			runtime.ReadMemStats(&m1)
			_, err := new.EncodeMessageFromUMHInstanceToUser(largeMessage)
			Expect(err).NotTo(HaveOccurred())
			runtime.ReadMemStats(&m2)

			newAllocs := m2.Mallocs - m1.Mallocs
			newBytes := m2.TotalAlloc - m1.TotalAlloc

			// Measure old implementation
			runtime.GC()
			runtime.ReadMemStats(&m1)
			_, err = old.EncodeMessageFromUMHInstanceToUser(largeMessage)
			Expect(err).NotTo(HaveOccurred())
			runtime.ReadMemStats(&m2)

			oldAllocs := m2.Mallocs - m1.Mallocs
			oldBytes := m2.TotalAlloc - m1.TotalAlloc

			experiment.RecordValue("new-allocs", float64(newAllocs))
			experiment.RecordValue("old-allocs", float64(oldAllocs))
			experiment.RecordValue("new-bytes", float64(newBytes))
			experiment.RecordValue("old-bytes", float64(oldBytes))

			// Print detailed allocation statistics
			By(fmt.Sprintf("Memory Usage Statistics:"))
			By(fmt.Sprintf("New Implementation:"))
			By(fmt.Sprintf("  Allocations: %d", newAllocs))
			By(fmt.Sprintf("  Bytes: %.2f KB", float64(newBytes)/1024))
			By(fmt.Sprintf("Old Implementation:"))
			By(fmt.Sprintf("  Allocations: %d", oldAllocs))
			By(fmt.Sprintf("  Bytes: %.2f KB", float64(oldBytes)/1024))
			By(fmt.Sprintf("Improvement:"))
			By(fmt.Sprintf("  Allocations: %.2f%%", (1-float64(newAllocs)/float64(oldAllocs))*100))
			By(fmt.Sprintf("  Memory: %.2f%%", (1-float64(newBytes)/float64(oldBytes))*100))

			// Allow for some variance but expect improvements
			maxAllowedAllocs := oldAllocs + uint64(float64(oldAllocs)*0.3) // Allow 30% more allocations
			maxAllowedBytes := oldBytes + uint64(float64(oldBytes)*0.3)    // Allow 30% more bytes

			Expect(newAllocs).To(BeNumerically("<=", maxAllowedAllocs),
				"New implementation should not use significantly more allocations")
			Expect(newBytes).To(BeNumerically("<=", maxAllowedBytes),
				"New implementation should not use significantly more memory")
		})
	})

	Context("Decoding Performance", func() {
		var encodedLargeMessage, encodedSmallMessage string

		BeforeEach(func() {
			var err error
			encodedLargeMessage, err = new.EncodeMessageFromUMHInstanceToUser(largeMessage)
			Expect(err).NotTo(HaveOccurred())
			encodedSmallMessage, err = new.EncodeMessageFromUMHInstanceToUser(smallMessage)
			Expect(err).NotTo(HaveOccurred())

			// Add warmup phase specific to decoding
			By("Warming up decoders")
			for i := 0; i < 1000; i++ {
				_, _ = new.DecodeMessageFromUMHInstanceToUser(encodedSmallMessage)
				_, _ = new.DecodeMessageFromUMHInstanceToUser(encodedLargeMessage)
				_, _ = old.DecodeMessageFromUMHInstanceToUser(encodedSmallMessage)
				_, _ = old.DecodeMessageFromUMHInstanceToUser(encodedLargeMessage)
			}
			runtime.GC() // Clean up after warmup
		})

		It("measures decoding performance improvements", func() {
			// Small messages
			experiment.Sample(func(idx int) {
				runtime.GC() // Ensure clean state before each sample
				experiment.MeasureDuration("new-small-decode", func() {
					for i := 0; i < 100; i++ { // Batch decode for more realistic measurement
						_, err := new.DecodeMessageFromUMHInstanceToUser(encodedSmallMessage)
						Expect(err).NotTo(HaveOccurred())
					}
				})
			}, gmeasure.SamplingConfig{N: 100, Duration: time.Second * 5})

			experiment.Sample(func(idx int) {
				runtime.GC() // Ensure clean state before each sample
				experiment.MeasureDuration("old-small-decode", func() {
					for i := 0; i < 100; i++ {
						_, err := old.DecodeMessageFromUMHInstanceToUser(encodedSmallMessage)
						Expect(err).NotTo(HaveOccurred())
					}
				})
			}, gmeasure.SamplingConfig{N: 100, Duration: time.Second * 5})

			// Large messages
			experiment.Sample(func(idx int) {
				runtime.GC() // Ensure clean state before each sample
				experiment.MeasureDuration("new-large-decode", func() {
					for i := 0; i < 100; i++ {
						_, err := new.DecodeMessageFromUMHInstanceToUser(encodedLargeMessage)
						Expect(err).NotTo(HaveOccurred())
					}
				})
			}, gmeasure.SamplingConfig{N: 100, Duration: time.Second * 5})

			experiment.Sample(func(idx int) {
				runtime.GC() // Ensure clean state before each sample
				experiment.MeasureDuration("old-large-decode", func() {
					for i := 0; i < 100; i++ {
						_, err := old.DecodeMessageFromUMHInstanceToUser(encodedLargeMessage)
						Expect(err).NotTo(HaveOccurred())
					}
				})
			}, gmeasure.SamplingConfig{N: 100, Duration: time.Second * 5})

			// Verify improvements and print detailed statistics
			for _, size := range []string{"small", "large"} {
				newStats := experiment.GetStats(fmt.Sprintf("new-%s-decode", size))
				oldStats := experiment.GetStats(fmt.Sprintf("old-%s-decode", size))

				medianNew := newStats.DurationFor(gmeasure.StatMedian)
				medianOld := oldStats.DurationFor(gmeasure.StatMedian)

				Expect(float64(medianNew)).To(BeNumerically("<", float64(medianOld)*1.5))

				improvement := float64(medianOld-medianNew) / float64(medianOld) * 100
				experiment.RecordValue(fmt.Sprintf("%s-decode-improvement-%%", size), improvement)

				// Print detailed statistics for this size
				By(fmt.Sprintf("Performance Statistics (%s):", size))
				By(fmt.Sprintf("New Implementation:"))
				By(fmt.Sprintf("  Median: %v", medianNew))
				By(fmt.Sprintf("  Mean: %v", newStats.DurationFor(gmeasure.StatMean)))
				By(fmt.Sprintf("  StdDev: %v", newStats.DurationFor(gmeasure.StatStdDev)))
				By(fmt.Sprintf("Old Implementation:"))
				By(fmt.Sprintf("  Median: %v", medianOld))
				By(fmt.Sprintf("  Mean: %v", oldStats.DurationFor(gmeasure.StatMean)))
				By(fmt.Sprintf("  StdDev: %v", oldStats.DurationFor(gmeasure.StatStdDev)))
				By(fmt.Sprintf("Improvement: %.2f%%", improvement))
			}
		})

		It("measures decoding memory allocations", func() {
			runtime.GC()
			var m1, m2 runtime.MemStats

			// Additional warmup specific to allocation testing
			for i := 0; i < 100; i++ {
				_, _ = new.DecodeMessageFromUMHInstanceToUser(encodedLargeMessage)
				_, _ = old.DecodeMessageFromUMHInstanceToUser(encodedLargeMessage)
			}
			runtime.GC()

			// Measure new implementation with large message
			runtime.ReadMemStats(&m1)
			_, err := new.DecodeMessageFromUMHInstanceToUser(encodedLargeMessage)
			Expect(err).NotTo(HaveOccurred())
			runtime.ReadMemStats(&m2)

			newAllocs := m2.Mallocs - m1.Mallocs
			newBytes := m2.TotalAlloc - m1.TotalAlloc

			// Measure old implementation
			runtime.GC()
			runtime.ReadMemStats(&m1)
			_, err = old.DecodeMessageFromUMHInstanceToUser(encodedLargeMessage)
			Expect(err).NotTo(HaveOccurred())
			runtime.ReadMemStats(&m2)

			oldAllocs := m2.Mallocs - m1.Mallocs
			oldBytes := m2.TotalAlloc - m1.TotalAlloc

			experiment.RecordValue("new-decode-allocs", float64(newAllocs))
			experiment.RecordValue("old-decode-allocs", float64(oldAllocs))
			experiment.RecordValue("new-decode-bytes", float64(newBytes))
			experiment.RecordValue("old-decode-bytes", float64(oldBytes))

			// Print detailed allocation statistics
			By(fmt.Sprintf("Memory Usage Statistics (Decoding):"))
			By(fmt.Sprintf("New Implementation:"))
			By(fmt.Sprintf("  Allocations: %d", newAllocs))
			By(fmt.Sprintf("  Bytes: %.2f KB", float64(newBytes)/1024))
			By(fmt.Sprintf("Old Implementation:"))
			By(fmt.Sprintf("  Allocations: %d", oldAllocs))
			By(fmt.Sprintf("  Bytes: %.2f KB", float64(oldBytes)/1024))
			By(fmt.Sprintf("Improvement:"))
			By(fmt.Sprintf("  Allocations: %.2f%%", (1-float64(newAllocs)/float64(oldAllocs))*100))
			By(fmt.Sprintf("  Memory: %.2f%%", (1-float64(newBytes)/float64(oldBytes))*100))

			// Allow for some variance but expect improvements
			maxAllowedAllocs := oldAllocs + uint64(float64(oldAllocs)*0.1) // Allow 10% more allocations
			maxAllowedBytes := oldBytes + uint64(float64(oldBytes)*0.1)    // Allow 10% more bytes

			Expect(newAllocs).To(BeNumerically("<=", maxAllowedAllocs),
				"New implementation should not use significantly more allocations")
			Expect(newBytes).To(BeNumerically("<=", maxAllowedBytes),
				"New implementation should not use significantly more memory")
		})
	})
})

var _ = Describe("Batch Processing Performance", Serial, Label("measurement"), func() {
	BeforeEach(func() {
		Skip("Skipping batch processing performance tests due to unreliable test runners")
	})

	var (
		messages        []models.UMHMessageContent
		encodedMessages []string
		experiment      *gmeasure.Experiment
		messageCount    = 1000 // Number of messages to process
	)

	BeforeEach(func() {
		experiment = gmeasure.NewExperiment("Batch Processing Performance")
		AddReportEntry(experiment.Name, experiment)

		// Pre-generate messages of varying sizes
		messages = make([]models.UMHMessageContent, messageCount)
		sizes := []int{
			500,                          // 500B - below compression threshold
			new.CompressionThreshold - 1, // Just below threshold
			new.CompressionThreshold + 1, // Just above threshold
			5 * 1024,                     // 5KB
			50 * 1024,                    // 50KB
			500 * 1024,                   // 500KB
			2 * 1024 * 1024,              // 2MB
		}

		By("Preparing test messages")
		for i := 0; i < messageCount; i++ {
			size := sizes[i%len(sizes)] // Cycle through different sizes
			payload := make([]byte, size)
			for j := range payload {
				payload[j] = byte(65 + (j % 26)) // Fill with repeating A-Z
			}

			messages[i] = models.UMHMessageContent{
				MessageType: models.Status,
				Payload:     string(payload),
			}
		}

		// Pre-encode messages with new implementation for decode testing
		By("Pre-encoding messages")
		encodedMessages = make([]string, messageCount)
		for i, msg := range messages {
			encoded, err := new.EncodeMessageFromUMHInstanceToUser(msg)
			Expect(err).NotTo(HaveOccurred())
			encodedMessages[i] = encoded
		}
	})

	Context("Batch Processing", func() {
		It("measures batch encoding performance", func() {
			runtime.GC()

			// Measure new implementation
			experiment.Sample(func(idx int) {
				experiment.MeasureDuration("new-batch-encode", func() {
					for _, msg := range messages {
						_, err := new.EncodeMessageFromUMHInstanceToUser(msg)
						Expect(err).NotTo(HaveOccurred())
					}
				})
			}, gmeasure.SamplingConfig{N: 10, Duration: time.Minute})

			// Measure old implementation
			experiment.Sample(func(idx int) {
				experiment.MeasureDuration("old-batch-encode", func() {
					for _, msg := range messages {
						_, err := old.EncodeMessageFromUMHInstanceToUser(msg)
						Expect(err).NotTo(HaveOccurred())
					}
				})
			}, gmeasure.SamplingConfig{N: 10, Duration: time.Minute})

			// Compare results
			newStats := experiment.GetStats("new-batch-encode")
			oldStats := experiment.GetStats("old-batch-encode")

			medianNew := newStats.DurationFor(gmeasure.StatMedian)
			medianOld := oldStats.DurationFor(gmeasure.StatMedian)

			By(fmt.Sprintf("New implementation median: %v", medianNew))
			By(fmt.Sprintf("Old implementation median: %v", medianOld))
			By(fmt.Sprintf("Messages processed: %d", messageCount))
			By(fmt.Sprintf("Messages/second (new): %.2f", float64(messageCount)/medianNew.Seconds()))
			By(fmt.Sprintf("Messages/second (old): %.2f", float64(messageCount)/medianOld.Seconds()))

			Expect(medianNew).To(BeNumerically("<", medianOld))
			improvement := float64(medianOld-medianNew) / float64(medianOld) * 100
			experiment.RecordValue("Batch encode improvement %", improvement)
		})

		It("measures batch decoding performance", func() {
			runtime.GC()

			// Add a more thorough warmup phase
			By("Warming up decoders")
			warmupBatchSize := len(encodedMessages) / 10 // Use 10% of messages for warmup
			for i := 0; i < 3; i++ {                     // Run warmup 3 times
				for _, encoded := range encodedMessages[:warmupBatchSize] {
					_, _ = new.DecodeMessageFromUMHInstanceToUser(encoded)
					_, _ = old.DecodeMessageFromUMHInstanceToUser(encoded)
				}
			}
			runtime.GC() // Clean up after warmup

			// Measure new implementation
			experiment.Sample(func(idx int) {
				experiment.MeasureDuration("new-batch-decode", func() {
					for _, encoded := range encodedMessages {
						_, err := new.DecodeMessageFromUMHInstanceToUser(encoded)
						Expect(err).NotTo(HaveOccurred())
					}
				})
			}, gmeasure.SamplingConfig{N: 10, Duration: time.Minute})

			// Measure old implementation
			experiment.Sample(func(idx int) {
				experiment.MeasureDuration("old-batch-decode", func() {
					for _, encoded := range encodedMessages {
						_, err := old.DecodeMessageFromUMHInstanceToUser(encoded)
						Expect(err).NotTo(HaveOccurred())
					}
				})
			}, gmeasure.SamplingConfig{N: 10, Duration: time.Minute})

			// Compare results
			newStats := experiment.GetStats("new-batch-decode")
			oldStats := experiment.GetStats("old-batch-decode")

			medianNew := newStats.DurationFor(gmeasure.StatMedian)
			medianOld := oldStats.DurationFor(gmeasure.StatMedian)

			By(fmt.Sprintf("New implementation median: %v", medianNew))
			By(fmt.Sprintf("Old implementation median: %v", medianOld))
			By(fmt.Sprintf("Messages processed: %d", messageCount))
			By(fmt.Sprintf("Messages/second (new): %.2f", float64(messageCount)/medianNew.Seconds()))
			By(fmt.Sprintf("Messages/second (old): %.2f", float64(messageCount)/medianOld.Seconds()))

			// Allow for some variance in batch processing
			maxAllowedTime := time.Duration(float64(medianOld) * 1.25) // Allow 25% variance instead of 15%
			Expect(medianNew).To(BeNumerically("<=", maxAllowedTime),
				"New implementation should not be significantly slower")
		})

		It("measures batch memory usage", func() {
			runtime.GC()
			var m1, m2 runtime.MemStats

			// Warm up
			for i := 0; i < 3; i++ {
				for _, msg := range messages[:10] { // Use first 10 messages for warmup
					_, _ = new.EncodeMessageFromUMHInstanceToUser(msg)
					_, _ = old.EncodeMessageFromUMHInstanceToUser(msg)
				}
			}

			runtime.GC()

			// Measure new implementation
			runtime.ReadMemStats(&m1)
			for _, msg := range messages {
				_, err := new.EncodeMessageFromUMHInstanceToUser(msg)
				Expect(err).NotTo(HaveOccurred())
			}
			runtime.ReadMemStats(&m2)

			newAllocs := m2.Mallocs - m1.Mallocs
			newBytes := m2.TotalAlloc - m1.TotalAlloc

			// Measure old implementation
			runtime.GC()
			runtime.ReadMemStats(&m1)
			for _, msg := range messages {
				_, err := old.EncodeMessageFromUMHInstanceToUser(msg)
				Expect(err).NotTo(HaveOccurred())
			}
			runtime.ReadMemStats(&m2)

			oldAllocs := m2.Mallocs - m1.Mallocs
			oldBytes := m2.TotalAlloc - m1.TotalAlloc

			By(fmt.Sprintf("Batch processing - New implementation: %d allocs (%.2f MB)",
				newAllocs, float64(newBytes)/(1024*1024)))
			By(fmt.Sprintf("Batch processing - Old implementation: %d allocs (%.2f MB)",
				oldAllocs, float64(oldBytes)/(1024*1024)))
			By(fmt.Sprintf("Memory reduction: %.2f%%",
				(1-float64(newBytes)/float64(oldBytes))*100))

			experiment.RecordValue("new-batch-allocs", float64(newAllocs))
			experiment.RecordValue("old-batch-allocs", float64(oldAllocs))
			experiment.RecordValue("new-batch-bytes", float64(newBytes))
			experiment.RecordValue("old-batch-bytes", float64(oldBytes))

			// Allow for 20% variance in batch processing
			maxAllowedAllocs := oldAllocs + uint64(float64(oldAllocs)/5) // Allow 20% variance in batch processing
			maxAllowedBytes := oldBytes + uint64(float64(oldBytes)/5)

			Expect(newAllocs).To(BeNumerically("<=", maxAllowedAllocs),
				"Batch processing should not use significantly more allocations")
			Expect(newBytes).To(BeNumerically("<=", maxAllowedBytes),
				"Batch processing should not use significantly more memory")
		})
	})
})

var _ = Describe("Thread Safety", func() {
	var (
		message models.UMHMessageContent
		wg      sync.WaitGroup
	)

	BeforeEach(func() {
		message = models.UMHMessageContent{
			MessageType: models.Status,
			Payload:     strings.Repeat("test payload", 1000), // Large enough to trigger compression
		}
	})

	It("should handle concurrent encoding safely", func() {
		const numGoroutines = 50
		const iterationsPerGoroutine = 100
		results := make(chan error, numGoroutines*iterationsPerGoroutine)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < iterationsPerGoroutine; j++ {
					encoded, err := new.EncodeMessageFromUMHInstanceToUser(message)
					if err != nil {
						results <- err
						return
					}
					// Verify the encoded message can be decoded
					_, err = new.DecodeMessageFromUMHInstanceToUser(encoded)
					if err != nil {
						results <- err
						return
					}
				}
			}()
		}

		// Wait for all goroutines to complete
		wg.Wait()
		close(results)

		// Check for any errors
		for err := range results {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should handle concurrent compression/decompression safely", func() {
		const numGoroutines = 50
		const iterationsPerGoroutine = 100
		results := make(chan error, numGoroutines*iterationsPerGoroutine)

		// Create a large string that will definitely be compressed
		largeString := strings.Repeat("test data for compression", 1000)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < iterationsPerGoroutine; j++ {
					// Test compression
					compressed, err := new.Compress([]byte(largeString))
					if err != nil {
						results <- fmt.Errorf("compression error: %v", err)
						return
					}

					// Test decompression
					decompressed, err := new.Decompress(compressed)
					if err != nil {
						results <- fmt.Errorf("decompression error: %v", err)
						return
					}

					// Verify the result
					if string(decompressed) != largeString {
						results <- fmt.Errorf("data mismatch after compression/decompression")
						return
					}
				}
			}()
		}

		// Wait for all goroutines to complete
		wg.Wait()
		close(results)

		// Check for any errors
		for err := range results {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should handle concurrent pool usage safely", func() {
		const numGoroutines = 50
		const iterationsPerGoroutine = 100
		results := make(chan error, numGoroutines*iterationsPerGoroutine)

		// Test with varying message sizes to stress the pools
		messageSizes := []int{
			100,                           // Small message
			new.CompressionThreshold - 10, // Just below threshold
			new.CompressionThreshold + 10, // Just above threshold
			new.CompressionThreshold * 2,  // Large message
		}

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(routineNum int) {
				defer wg.Done()
				for j := 0; j < iterationsPerGoroutine; j++ {
					// Use different message sizes to exercise different paths
					size := messageSizes[j%len(messageSizes)]
					msg := models.UMHMessageContent{
						MessageType: models.Status,
						Payload:     strings.Repeat("x", size),
					}

					// Encode
					encoded, err := new.EncodeMessageFromUMHInstanceToUser(msg)
					if err != nil {
						results <- fmt.Errorf("encoding error: %v", err)
						return
					}

					// Decode
					decoded, err := new.DecodeMessageFromUMHInstanceToUser(encoded)
					if err != nil {
						results <- fmt.Errorf("decoding error: %v", err)
						return
					}

					// Verify
					if decoded.Payload != msg.Payload {
						results <- fmt.Errorf("data mismatch in routine %d, iteration %d", routineNum, j)
						return
					}
				}
			}(i)
		}

		// Wait for all goroutines to complete
		wg.Wait()
		close(results)

		// Check for any errors
		for err := range results {
			Expect(err).NotTo(HaveOccurred())
		}
	})

})

var _ = Describe("PPROF tests", func() {
	BeforeEach(func() {
		Skip("Skipping PPROF tests due to unreliable test runners")
	})

	const iterations = 100_000
	It("should measure compression performance", func() {
		runtime.GC()
		defer runtime.GC()
		cpuFile, err := os.Create("cpu-compression.prof")
		Expect(err).NotTo(HaveOccurred())
		defer cpuFile.Close()

		err = pprof.StartCPUProfile(cpuFile)
		Expect(err).NotTo(HaveOccurred())
		defer pprof.StopCPUProfile()
		data := []byte(strings.Repeat("test data for compression", 1000))

		for i := 0; i < iterations; i++ {
			result, err := new.Compress(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		}

	})

	It("should measure decompression performance", func() {
		data, err := new.Compress([]byte(strings.Repeat("test data for compression", 1000)))
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(BeNil())

		runtime.GC()
		defer runtime.GC()
		cpuFile, err := os.Create("cpu-decompression.prof")
		Expect(err).NotTo(HaveOccurred())
		defer cpuFile.Close()

		err = pprof.StartCPUProfile(cpuFile)
		Expect(err).NotTo(HaveOccurred())
		defer pprof.StopCPUProfile()

		for i := 0; i < iterations; i++ {
			result, err := new.Decompress(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		}

	})
})
