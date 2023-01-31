package t_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestExportEnvVariables(t *testing.T) {
	path, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Go two levels up and into cmd
	path = path[:len(path)-len("test\\docs")] + "cmd"

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			t.Logf("=== %s ===", dirEntry.Name())
			envVars := scanDir(t, path+"\\"+dirEntry.Name())
			for _, envVar := range envVars {
				t.Logf("\t%s", envVar)
			}
		}
	}
}

var re = regexp.MustCompile(`os.((LookupEnv)|(GetEnv))\("(.+)"\)`)

func scanDir(t *testing.T, path string) []string {

	envVariables := make([]string, 0)

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
				envVariables = append(envVariables, matches[4])
			}
		}
		return nil
	})

	return envVariables
}
