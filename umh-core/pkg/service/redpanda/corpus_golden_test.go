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

package redpanda

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// The corpus is one config per shape a real config.yaml can take. It lives with
// the config package because that is what parses it; the goldens live here
// because this is where subject names are derived.
const (
	corpusDir = "../../config/testdata/corpus"
	goldenDir = "testdata/golden"
)

// corpusSnapshot is what must not change. Subject names are what benthos looks
// up, so a moved name silently changes which messages get validated. Skip
// prefixes are what protects an existing subject from deletion, so they are part
// of the observable behaviour too — dropping one deletes live schemas.
type corpusSnapshot struct {
	TranslateError string   `json:"translateError,omitempty"`
	Subjects       []string `json:"subjects"`
	SkipPrefixes   []string `json:"skipPrefixes"`
}

// corpusFiles is read at tree-construction time so DescribeTable can enumerate it.
func corpusFiles() []string {
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		panic("cannot read corpus dir " + corpusDir + ": " + err.Error())
	}

	names := make([]string, 0, len(entries))

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, e.Name())
		}
	}

	sort.Strings(names)

	return names
}

func snapshotOf(name string) corpusSnapshot {
	data, err := os.ReadFile(filepath.Join(corpusDir, name))
	Expect(err).NotTo(HaveOccurred(), "reading corpus file")

	ctx := context.Background()

	cfg, err := config.ParseConfig(data, ctx, false)
	Expect(err).NotTo(HaveOccurred(), "corpus file must parse: "+name)

	registry := NewSchemaRegistry()

	subjects, skipPrefixes, translateErr := registry.translateToSchemas(
		ctx, cfg.DataModels, cfg.DataContracts, cfg.PayloadShapes,
	)

	snap := corpusSnapshot{
		Subjects:     make([]string, 0, len(subjects)),
		SkipPrefixes: append([]string(nil), skipPrefixes...),
	}

	for subject := range subjects {
		snap.Subjects = append(snap.Subjects, string(subject))
	}

	sort.Strings(snap.Subjects)
	sort.Strings(snap.SkipPrefixes)

	if translateErr != nil {
		snap.TranslateError = translateErr.Error()
	}

	return snap
}

// tableArgs is the body function followed by one entry per corpus file. Go does
// not allow mixing a fixed argument with a spread, so they are built together.
func tableArgs(body func(string)) []any {
	files := corpusFiles()
	args := make([]any, 0, len(files)+1)
	args = append(args, body)

	for _, f := range files {
		args = append(args, Entry(f, f))
	}

	return args
}

var _ = Describe("corpus goldens", func() {
	// Regenerate with UPDATE_GOLDEN=1. Committing a golden change is the explicit
	// act of declaring an intended behaviour change — it should never happen as a
	// side effect of a refactor.
	updating := os.Getenv("UPDATE_GOLDEN") == "1"

	check := func(name string) {
		got := snapshotOf(name)

		encoded, err := json.MarshalIndent(got, "", "  ")
		Expect(err).NotTo(HaveOccurred())

		encoded = append(encoded, '\n')
		goldenPath := filepath.Join(goldenDir, strings.TrimSuffix(name, ".yaml")+".json")

		if updating {
			Expect(os.MkdirAll(goldenDir, 0o755)).To(Succeed())
			Expect(os.WriteFile(goldenPath, encoded, 0o644)).To(Succeed())

			return
		}

		want, err := os.ReadFile(goldenPath)
		Expect(err).NotTo(HaveOccurred(),
			"missing golden for "+name+" - regenerate with UPDATE_GOLDEN=1")
		Expect(string(encoded)).To(Equal(string(want)),
			"subject names or skip prefixes moved for "+name)
	}

	DescribeTable("subject names and skip prefixes are unchanged", tableArgs(check)...)
})
