package s6

import (
	"strings"

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
		// Mock parseCommandLine function with the same logic as in s6.go
		parseCommandLine := func(cmdLine string) []string {
			var cmdParts []string
			var currentPart strings.Builder
			inQuote := false
			quoteChar := byte(0)

			for i := 0; i < len(cmdLine); i++ {
				if cmdLine[i] == '"' || cmdLine[i] == '\'' {
					if inQuote && cmdLine[i] == quoteChar {
						inQuote = false
						quoteChar = 0
					} else if !inQuote {
						inQuote = true
						quoteChar = cmdLine[i]
					} else {
						// This is a different quote character inside a quote
						currentPart.WriteByte(cmdLine[i])
					}
					continue
				}

				if cmdLine[i] == ' ' && !inQuote {
					if currentPart.Len() > 0 {
						cmdParts = append(cmdParts, currentPart.String())
						currentPart.Reset()
					}
				} else {
					currentPart.WriteByte(cmdLine[i])
				}
			}

			if currentPart.Len() > 0 {
				cmdParts = append(cmdParts, currentPart.String())
			}

			return cmdParts
		}

		DescribeTable("splitting command line respecting quotes",
			func(cmdLine string, expected []string) {
				result := parseCommandLine(cmdLine)
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
