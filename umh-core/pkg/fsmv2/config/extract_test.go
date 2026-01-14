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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("Extract", func() {
	type TestConfig struct {
		IP      string `json:"IP"`
		PORT    int    `json:"PORT"`
		Enabled bool   `json:"enabled"`
	}

	Describe("Extract[T]", func() {
		Context("with valid extraction", func() {
			It("should extract to typed struct with matching fields", func() {
				vars := config.VariableBundle{
					User: map[string]any{
						"IP":      "192.168.1.100",
						"PORT":    float64(502), // JSON numbers are float64
						"enabled": true,
					},
				}

				result, err := config.Extract[TestConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.IP).To(Equal("192.168.1.100"))
				Expect(result.PORT).To(Equal(502))
				Expect(result.Enabled).To(BeTrue())
			})

			It("should handle nested struct types", func() {
				type NestedConfig struct {
					Connection struct {
						Host string `json:"host"`
						Port int    `json:"port"`
					} `json:"connection"`
					Timeout int `json:"timeout"`
				}

				vars := config.VariableBundle{
					User: map[string]any{
						"connection": map[string]any{
							"host": "localhost",
							"port": float64(8080),
						},
						"timeout": float64(5000),
					},
				}

				result, err := config.Extract[NestedConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.Connection.Host).To(Equal("localhost"))
				Expect(result.Connection.Port).To(Equal(8080))
				Expect(result.Timeout).To(Equal(5000))
			})
		})

		Context("with missing fields", func() {
			It("should set missing fields to zero values", func() {
				vars := config.VariableBundle{
					User: map[string]any{
						"IP": "192.168.1.100",
						// PORT and enabled are missing
					},
				}

				result, err := config.Extract[TestConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.IP).To(Equal("192.168.1.100"))
				Expect(result.PORT).To(Equal(0))        // zero value for int
				Expect(result.Enabled).To(BeFalse())    // zero value for bool
			})
		})

		Context("with extra fields", func() {
			It("should ignore extra fields in VariableBundle", func() {
				vars := config.VariableBundle{
					User: map[string]any{
						"IP":            "192.168.1.100",
						"PORT":          float64(502),
						"enabled":       true,
						"extra_field":   "should be ignored",
						"another_extra": float64(12345),
					},
				}

				result, err := config.Extract[TestConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.IP).To(Equal("192.168.1.100"))
				Expect(result.PORT).To(Equal(502))
				Expect(result.Enabled).To(BeTrue())
			})
		})

		Context("with invalid unmarshal", func() {
			It("should return error on type mismatch", func() {
				vars := config.VariableBundle{
					User: map[string]any{
						"IP":   "192.168.1.100",
						"PORT": "not-a-number", // string instead of int
					},
				}

				_, err := config.Extract[TestConfig](vars)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unmarshal"))
			})

			It("should return error when extracting to incompatible type", func() {
				type StrictConfig struct {
					Value int `json:"value"`
				}

				vars := config.VariableBundle{
					User: map[string]any{
						"value": map[string]any{"nested": "object"}, // object instead of int
					},
				}

				_, err := config.Extract[StrictConfig](vars)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unmarshal"))
			})
		})

		Context("with empty VariableBundle", func() {
			It("should extract to zero-value struct", func() {
				vars := config.VariableBundle{}

				result, err := config.Extract[TestConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.IP).To(Equal(""))
				Expect(result.PORT).To(Equal(0))
				Expect(result.Enabled).To(BeFalse())
			})

			It("should extract empty User map to zero-value struct", func() {
				vars := config.VariableBundle{
					User: map[string]any{},
				}

				result, err := config.Extract[TestConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.IP).To(Equal(""))
				Expect(result.PORT).To(Equal(0))
				Expect(result.Enabled).To(BeFalse())
			})
		})

		Context("with nil User map", func() {
			It("should handle nil User map gracefully", func() {
				vars := config.VariableBundle{
					User: nil,
				}

				result, err := config.Extract[TestConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.IP).To(Equal(""))
				Expect(result.PORT).To(Equal(0))
				Expect(result.Enabled).To(BeFalse())
			})
		})

		Context("with pointer fields", func() {
			It("should handle pointer fields correctly", func() {
				type PointerConfig struct {
					Name  *string `json:"name"`
					Count *int    `json:"count"`
				}

				name := "test-name"
				count := 42
				vars := config.VariableBundle{
					User: map[string]any{
						"name":  name,
						"count": float64(count),
					},
				}

				result, err := config.Extract[PointerConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.Name).ToNot(BeNil())
				Expect(*result.Name).To(Equal("test-name"))
				Expect(result.Count).ToNot(BeNil())
				Expect(*result.Count).To(Equal(42))
			})

			It("should leave pointer fields nil when not present", func() {
				type PointerConfig struct {
					Name  *string `json:"name"`
					Count *int    `json:"count"`
				}

				vars := config.VariableBundle{
					User: map[string]any{},
				}

				result, err := config.Extract[PointerConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.Name).To(BeNil())
				Expect(result.Count).To(BeNil())
			})
		})

		Context("with slice fields", func() {
			It("should extract slice fields correctly", func() {
				type SliceConfig struct {
					Tags   []string `json:"tags"`
					Values []int    `json:"values"`
				}

				vars := config.VariableBundle{
					User: map[string]any{
						"tags":   []any{"tag1", "tag2", "tag3"},
						"values": []any{float64(1), float64(2), float64(3)},
					},
				}

				result, err := config.Extract[SliceConfig](vars)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.Tags).To(Equal([]string{"tag1", "tag2", "tag3"}))
				Expect(result.Values).To(Equal([]int{1, 2, 3}))
			})
		})
	})

	Describe("MustExtract[T]", func() {
		Context("on success", func() {
			It("should return value on successful extraction", func() {
				vars := config.VariableBundle{
					User: map[string]any{
						"IP":      "192.168.1.100",
						"PORT":    float64(502),
						"enabled": true,
					},
				}

				result := config.MustExtract[TestConfig](vars)

				Expect(result.IP).To(Equal("192.168.1.100"))
				Expect(result.PORT).To(Equal(502))
				Expect(result.Enabled).To(BeTrue())
			})

			It("should return zero-value struct for empty bundle", func() {
				vars := config.VariableBundle{}

				result := config.MustExtract[TestConfig](vars)

				Expect(result.IP).To(Equal(""))
				Expect(result.PORT).To(Equal(0))
				Expect(result.Enabled).To(BeFalse())
			})
		})

		Context("on error", func() {
			It("should panic on type mismatch", func() {
				vars := config.VariableBundle{
					User: map[string]any{
						"PORT": "not-a-number",
					},
				}

				Expect(func() {
					config.MustExtract[TestConfig](vars)
				}).To(Panic())
			})

			It("should panic with error containing unmarshal message", func() {
				vars := config.VariableBundle{
					User: map[string]any{
						"PORT": "invalid",
					},
				}

				defer func() {
					r := recover()
					Expect(r).ToNot(BeNil())
					err, ok := r.(error)
					Expect(ok).To(BeTrue())
					Expect(err.Error()).To(ContainSubstring("unmarshal"))
				}()

				config.MustExtract[TestConfig](vars)
			})
		})
	})
})
