package safejson_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/safejson"
)

var _ = Describe("SafeJson (Unmarshal)", func() {
	It("decodes a json string", func() {
		jsonString := `{"key": "value"}`
		result := make(map[string]interface{})
		err := safejson.Unmarshal([]byte(jsonString), &result)
		Expect(err).To(BeNil())
		Expect(result["key"]).To(Equal("value"))
	})

	It("should not work with nil ptr receiver", func() {
		jsonString := `{"key": "value"}`
		var result map[string]interface{}
		err := safejson.Unmarshal([]byte(jsonString), result)
		Expect(err).ToNot(BeNil())
		Expect(result).To(BeNil())
	})
	It("decodes a complex json structure", func() {
		jsonString := `{"key1": "value1", "key2": {"nestedKey": "nestedValue"}}`
		result := make(map[string]interface{})
		err := safejson.Unmarshal([]byte(jsonString), &result)
		Expect(err).To(BeNil())
		Expect(result["key1"]).To(Equal("value1"))
		nestedMap := result["key2"].(map[string]interface{})
		Expect(nestedMap["nestedKey"]).To(Equal("nestedValue"))
	})

	It("handles invalid json gracefully", func() {
		jsonString := `{"key": "value"`
		result := make(map[string]interface{})
		err := safejson.Unmarshal([]byte(jsonString), &result)
		Expect(err).ToNot(BeNil())
	})

	It("handles empty json object", func() {
		jsonString := `{}`
		result := make(map[string]interface{})
		err := safejson.Unmarshal([]byte(jsonString), &result)
		Expect(err).To(BeNil())
		Expect(result).To(BeEmpty())
	})

	It("handles json array", func() {
		jsonString := `["value1", "value2"]`
		var result []interface{}
		err := safejson.Unmarshal([]byte(jsonString), &result)
		Expect(err).To(BeNil())
		Expect(result).To(HaveLen(2))
		Expect(result[0]).To(Equal("value1"))
		Expect(result[1]).To(Equal("value2"))
	})
})

type OuterJsonStruct struct {
	Inner    *InnerJsonStruct `json:"inner"`
	OtherKey *string          `json:"other_key,omitempty"`
}

type InnerJsonStruct struct {
	Key *string `json:"key"`
}

var _ = Describe("SafeJson (Marshal)", func() {
	It("encodes a json string", func() {
		jsonMap := map[string]interface{}{"key": "value"}
		result, err := safejson.Marshal(jsonMap)
		Expect(err).To(BeNil())
		Expect(string(result)).To(Equal(`{"key":"value"}`))
	})

	It("should work when we encode a nil map", func() {
		var jsonMap map[string]interface{}
		result, err := safejson.Marshal(jsonMap)
		Expect(err).To(BeNil())
		Expect(string(result)).To(Equal(`null`))
	})

	It("should work when we have a nil value in a nested structure", func() {
		var jsonStruct OuterJsonStruct
		jsonStruct.Inner = nil
		result, err := safejson.Marshal(jsonStruct)
		Expect(err).To(BeNil())
		Expect(string(result)).To(Equal(`{"inner":null}`))
	})

	It("encodes a nested json structure", func() {
		jsonMap := map[string]interface{}{
			"key1": "value1",
			"key2": map[string]interface{}{
				"nestedKey": "nestedValue",
			},
		}
		result, err := safejson.Marshal(jsonMap)
		Expect(err).To(BeNil())
		Expect(string(result)).To(MatchJSON(`{"key1":"value1","key2":{"nestedKey":"nestedValue"}}`))
	})

	It("handles empty map", func() {
		jsonMap := map[string]interface{}{}
		result, err := safejson.Marshal(jsonMap)
		Expect(err).To(BeNil())
		Expect(string(result)).To(Equal(`{}`))
	})

	It("handles nil slice", func() {
		var jsonSlice []interface{}
		result, err := safejson.Marshal(jsonSlice)
		Expect(err).To(BeNil())
		Expect(string(result)).To(Equal(`null`))
	})

	It("handles slice with nil elements", func() {
		jsonSlice := []interface{}{"value1", nil, "value3"}
		result, err := safejson.Marshal(jsonSlice)
		Expect(err).To(BeNil())
		Expect(string(result)).To(MatchJSON(`["value1",null,"value3"]`))
	})
})

var _ = Describe("SafeJson vs JSONTestSuite", func() {
	It("handles the test_parsing tests", func() {
		// test files are in test-data/test_parsing directory (relative to this file)
		testFiles := make(map[string]string)
		// Add test files to the list (read all .json files in the directory)
		dir, err := os.ReadDir("test-data/test_parsing")
		Expect(err).To(BeNil())
		Expect(dir).ToNot(BeNil())
		for _, file := range dir {
			// Read file to get the content
			fileContent, err := os.ReadFile("test-data/test_parsing/" + file.Name())
			Expect(err).To(BeNil())
			testFiles[file.Name()] = string(fileContent)
		}

		// Try to unmarshal the content of each file
		for fileName, fileContent := range testFiles {
			var safeJsonResult any
			err := safejson.Unmarshal([]byte(fileContent), &safeJsonResult)
			// If the file name begins with n_ we expect an error
			// If the file name begins with y_ we expect no error
			// If the file name begins with i_ there might be an error or not (we just don't care) [These are in here to check if the parser is panicking]
			if fileName[0] == 'n' {
				Expect(err).ToNot(BeNil())
			} else if fileName[0] == 'y' {
				Expect(err).To(BeNil())
			}
		}
	})

	It("handles the test_transform tests", func() {
		// test files are in test-data/test_transform directory (relative to this file)
		testFiles := make(map[string]string)
		// Add test files to the list (read all .json files in the directory)
		dir, err := os.ReadDir("test-data/test_transform")
		Expect(err).To(BeNil())
		Expect(dir).ToNot(BeNil())
		for _, file := range dir {
			// Read file to get the content
			fileContent, err := os.ReadFile("test-data/test_transform/" + file.Name())
			Expect(err).To(BeNil())
			testFiles[file.Name()] = string(fileContent)
		}

		// Try to unmarshal the content of each file
		for _, fileContent := range testFiles {
			var safeJsonResult any
			err := safejson.Unmarshal([]byte(fileContent), &safeJsonResult)
			// This must all succeed
			Expect(err).To(BeNil())
		}
	})
})

var _ = Describe("SafeJson (fuzz inputs)", func() {
	It("should match the results of safejson.Unmarshal and json.Unmarshal", func() {
		// Seed inputs
		var jsonInputs = []string{
			`{"key1": "value1", "key2": "value2"}`,
			`{"nested": {"inner_key": "inner_value"}}`,
			`{"list": [1, 2, 3, 4, 5]}`,
			`[{"array_object": "value"}, {"array_object2": 2}]`,
			`{"bool": true, "null_value": null}`,
			`{"int_value": 123, "float_value": 456.789}`,
			`{"complex_structure": {"list_of_dicts": [{"a": 1}, {"b": 2}]}}`,
			`{"empty_list": [], "empty_dict": {}}`,
			`{"unicode_string": "こんにちは世界", "escaped_chars": "\\t\\n\\""}`,
			`{"mixed_types": [1, "two", 3.0, {"four": 4}]}`,
			`{}`,
			`[]`,
			`{"long_string": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
			`{"negative_numbers": -123, "positive_numbers": 456}`,
			`{"special_characters": "!@#$%^&*()_+-="}`,
			`{"boolean_values": [true, false, true]}`,
			`{"nested_empty": {"a": {"b": {}}}}`,
			`{"simple_key": "simple_value"}`,
			`{"complex_numbers": [1e-09, -2.5, 3.1415]}`,
			`{"json_with_null": {"key": null}}`,
			`{"timestamp_ms":1724678619210,"\"\"\"PID401P01\"\"_OUT_StellwertAktuell\"":0.0922309011220932}`,
			`{"timestamp_ms":1724678619210,"\"\"\"PID401P01\"\"_OUT_StellwertAktuell\"":0,0922309011220932}`,
			`{"timestamp_ms":1724678619210,"\"\"\"PID4'01P01\"\"_OUT_StellwertAktuell\"":0,0922309011220932}`,
			`{"time'stamp_ms":1724678619210,"\"\"\"PID4'01P01\"\"_OUT_StellwertAktuell\"":0,0922309011220932}`,
			`{"time'stamp_ms":172467'8619210,"\"\"\"PID4'01P01\"\"_OUT_StellwertAktuell\"":0,0922309011220932}`,
		}

		for _, input := range jsonInputs {
			// Define target type
			target := new(map[string]interface{})

			// Unmarshal using safejson
			safeTarget := reflect.New(reflect.TypeOf(target).Elem()).Interface()
			err := safejson.Unmarshal([]byte(input), safeTarget)
			GinkgoT().Logf("SafeJSON Unmarshal Error: %v", err)

			// Unmarshal using standard library json
			stdlibTarget := reflect.New(reflect.TypeOf(target).Elem()).Interface()
			stdlibErr := json.Unmarshal([]byte(input), stdlibTarget)
			GinkgoT().Logf("Standard JSON Unmarshal Error: %v", stdlibErr)

			// If stdlib unmarshaling was successful, compare the results
			if stdlibErr == nil {
				Expect(safeTarget).To(Equal(stdlibTarget), "Mismatch between safejson.Unmarshal and json.Unmarshal results.")
			}
		}
	})
})

func FuzzUnmarshal(f *testing.F) {
	// Seed the fuzzer with some known JSON inputs
	var jsonInputs = []string{
		`{"key1": "value1", "key2": "value2"}`,
		`{"nested": {"inner_key": "inner_value"}}`,
		`{"list": [1, 2, 3, 4, 5]}`,
		`[{"array_object": "value"}, {"array_object2": 2}]`,
		`{"bool": true, "null_value": null}`,
		`{"int_value": 123, "float_value": 456.789}`,
		`{"complex_structure": {"list_of_dicts": [{"a": 1}, {"b": 2}]}}`,
		`{"empty_list": [], "empty_dict": {}}`,
		`{"unicode_string": "こんにちは世界", "escaped_chars": "\\t\\n\\""}`,
		`{"mixed_types": [1, "two", 3.0, {"four": 4}]}`,
		`{}`,
		`[]`,
		`{"long_string": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
		`{"negative_numbers": -123, "positive_numbers": 456}`,
		`{"special_characters": "!@#$%^&*()_+-="}`,
		`{"boolean_values": [true, false, true]}`,
		`{"nested_empty": {"a": {"b": {}}}}`,
		`{"simple_key": "simple_value"}`,
		`{"complex_numbers": [1e-09, -2.5, 3.1415]}`,
		`{"json_with_null": {"key": null}}`,
		`{"timestamp_ms":1724678619210,"\"\"\"PID401P01\"\"_OUT_StellwertAktuell\"":0.0922309011220932}`,
	}
	for _, input := range jsonInputs {
		f.Add([]byte(input))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Define different types to fuzz
		targets := []interface{}{
			new(map[string]interface{}),
		}

		for _, target := range targets {
			// Copy the target to avoid cross-test contamination
			targetCopy := reflect.New(reflect.TypeOf(target).Elem()).Interface()

			err := safejson.Unmarshal(data, targetCopy)
			if err != nil {
				t.Logf("Expected error: %v", err)
			}

			// Compare with stdlib json.Unmarshal if applicable
			stdlibDecoded := reflect.New(reflect.TypeOf(target).Elem()).Interface()
			stdlibErr := json.Unmarshal(data, stdlibDecoded)
			if stdlibErr == nil {
				if !reflect.DeepEqual(targetCopy, stdlibDecoded) {
					t.Errorf("Mismatch between Unmarshal and json.Unmarshal results.\nGot: %v\nWant: %v", targetCopy, stdlibDecoded)
				}
			}
		}
	})
}
