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

package s6_default

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config Parser", func() {
	Describe("Script Regex Parsing", func() {
		DescribeTable("command extraction",
			func(content, expectedCapture string) {
				cmdMatches := runScriptParser.FindStringSubmatch(content)
				Expect(cmdMatches).To(HaveLen(2), "Should have a capture group")
				// The first element is the full match, the second is the capture group
				if expectedCapture != "" {
					Expect(cmdMatches[1]).To(Equal(expectedCapture), "Captured content doesn't match")
				} else {
					Expect(cmdMatches[1]).To(BeEmpty(), "Expected empty capture")
				}
			},
			Entry("Simple command on same line",
				"fdmove -c 2 1 /usr/local/bin/benthos -c /config.yaml",
				"/usr/local/bin/benthos -c /config.yaml"),
			Entry("Command with options",
				"fdmove -c 2 1 /bin/sh -c \"echo hello\"",
				"/bin/sh -c \"echo hello\""),
			Entry("No command on the same line (captures next line with (?m) flag)",
				`fdmove -c 2 1
/usr/local/bin/benthos`,
				"/usr/local/bin/benthos"),
		)

		DescribeTable("environment variable extraction",
			func(content string, expectedEnvMap map[string]string) {
				envMatches := envVarParser.FindAllStringSubmatch(content, -1)
				extractedEnv := make(map[string]string)
				for _, match := range envMatches {
					if len(match) == 3 {
						extractedEnv[match[1]] = match[2]
					}
				}
				Expect(extractedEnv).To(Equal(expectedEnvMap), "Environment variables don't match")
			},
			Entry("Single environment variable",
				"export LOG_LEVEL DEBUG",
				map[string]string{"LOG_LEVEL": "DEBUG"}),
			Entry("Multiple environment variables",
				`export LOG_LEVEL DEBUG
export DATA_DIR /var/data
export COMPLEX_VAR value with spaces`,
				map[string]string{
					"LOG_LEVEL":   "DEBUG",
					"DATA_DIR":    "/var/data",
					"COMPLEX_VAR": "value with spaces",
				}),
			Entry("Environment variables with quotes",
				`export QUOTED_VAR "quoted value"
export SINGLE_QUOTED 'single quoted'`,
				map[string]string{
					"QUOTED_VAR":    "\"quoted value\"",
					"SINGLE_QUOTED": "'single quoted'",
				}),
		)
	})

	Describe("Command Line Parsing", func() {
		// parseCommandLine comes from s6.go

		DescribeTable("splitting command line respecting quotes",
			func(cmdLine string, expected []string) {
				result, err := parseCommandLine(cmdLine)
				Expect(err).ToNot(HaveOccurred(), "Command line parsing should not return an error")
				Expect(result).To(Equal(expected), "Command line parsing incorrect")
			},
			Entry("Simple command",
				"/usr/local/bin/benthos -c /config.yaml",
				[]string{"/usr/local/bin/benthos", "-c", "/config.yaml"}),
			Entry("Command with double quotes",
				`/bin/sh -c "echo hello world"`,
				[]string{"/bin/sh", "-c", "echo hello world"}),
			Entry("Command with single quotes",
				"/bin/sh -c 'grep -v test | sort'",
				[]string{"/bin/sh", "-c", "grep -v test | sort"}),
			Entry("Complex command with nested quotes",
				`/bin/sh -c "echo 'nested quotes' && grep test"`,
				[]string{"/bin/sh", "-c", "echo 'nested quotes' && grep test"}),
		)
	})
})
