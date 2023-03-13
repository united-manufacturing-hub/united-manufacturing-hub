// Copyright 2023 UMH Systems GmbH
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

package t_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

func TestExportEnvVariables(t *testing.T) {
	path, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Go two levels up and into cmd
	path = path[:len(path)-len("test\\docs")]

	ScanPath(t, err, path+"cmd")
	ScanPath(t, err, path+"internal")
	ScanPath(t, err, path+"pkg")
}

func ScanPath(t *testing.T, err error, path string) {
	t.Logf("==== %s ====", path)
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			t.Logf("\t=== %s ===", dirEntry.Name())
			envVars := scanDir(t, path+"\\"+dirEntry.Name())
			// Sort alphabetically
			sort.Strings(envVars)
			for _, envVar := range envVars {
				t.Logf("\t\t%s", envVar)
			}
		} else {
			t.Logf("\t=== %s ===", dirEntry.Name())
			envVars := extractVar(path+"\\"+dirEntry.Name(), t)
			// Sort alphabetically
			sort.Strings(envVars)
			for _, envVar := range envVars {
				t.Logf("\t\t%s", envVar)
			}
		}
	}
}

var re = regexp.MustCompile(`((logger\.New)|(os.((LookupEnv)|(Getenv))))\("(.+)"\)`)

func scanDir(t *testing.T, path string) []string {

	var envVariables []string

	filepath.WalkDir(path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		envVariables = append(envVariables, extractVar(path, t)...)
		return nil
	})

	return envVariables
}

func extractVar(path string, t *testing.T) []string {
	envVariables := make([]string, 0)
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	// Read file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) > 0 {
			envVariables = append(envVariables, matches[7])
		}
	}
	return envVariables
}
