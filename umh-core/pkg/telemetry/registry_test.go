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

package telemetry_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/telemetry"
)

var tagFormat = regexp.MustCompile(`^[a-z0-9_]+(::[a-z0-9_]+)+$`)

// domainVars is every exported domain var. Add one line per domain as the
// registry grows; the specs below then cover its entries without being edited.
func domainVars() []any {
	return []any{
		telemetry.Cpu,
		telemetry.Telemetry,
		telemetry.Transport,
	}
}

// collect walks a domain var by reflection and returns every Identifier under
// it, so a new entry is covered the moment it is generated.
func collect(value reflect.Value, found *[]telemetry.Identifier) {
	if value.Type() == reflect.TypeOf(telemetry.Identifier{}) {
		*found = append(*found, value.Interface().(telemetry.Identifier))

		return
	}

	if value.Kind() != reflect.Struct {
		return
	}

	for index := 0; index < value.NumField(); index++ {
		collect(value.Field(index), found)
	}
}

var _ = Describe("the generated registry", func() {
	var all []telemetry.Identifier

	BeforeEach(func() {
		all = nil
		for _, domain := range domainVars() {
			collect(reflect.ValueOf(domain), &all)
		}
	})

	It("holds at least one entry", func() {
		// Without this the three specs below pass over an empty slice, which they
		// would also do if collect stopped finding anything.
		Expect(all).NotTo(BeEmpty())
	})

	It("gives every entry a tag in the declared format", func() {
		for _, identifier := range all {
			Expect(tagFormat.MatchString(identifier.Tag)).To(BeTrue(), "tag %q", identifier.Tag)
		}
	})

	It("gives every entry a brief and a known severity", func() {
		for _, identifier := range all {
			Expect(identifier.Brief).NotTo(BeEmpty(), "tag %q has no brief", identifier.Tag)
			Expect(identifier.Severity).To(BeElementOf(telemetry.SeverityWarning, telemetry.SeverityError), "tag %q", identifier.Tag)
		}
	})

	It("gives every entry a unique tag", func() {
		seen := map[string]bool{}
		for _, identifier := range all {
			Expect(seen[identifier.Tag]).To(BeFalse(), "duplicate tag %q", identifier.Tag)
			seen[identifier.Tag] = true
		}
	})
})

// TestGeneratedFileIsCurrent shells out, so it is a plain Go test rather than a
// spec. An edited telemetry.yaml with a forgotten `make generate-telemetry`
// fails here rather than shipping an identifier nobody can report under.
func TestGeneratedFileIsCurrent(t *testing.T) {
	fresh := filepath.Join(t.TempDir(), "identifiers.gen.go")

	regenerate := exec.Command("go", "run", "./generate", "-in", "telemetry.yaml", "-out", fresh, "-package", "telemetry")
	if output, err := regenerate.CombinedOutput(); err != nil {
		t.Fatalf("regeneration failed: %v\n%s", err, output)
	}

	regenerated, err := os.ReadFile(fresh)
	if err != nil {
		t.Fatal(err)
	}

	committed, err := os.ReadFile("identifiers.gen.go")
	if err != nil {
		t.Fatal(err)
	}

	if string(regenerated) != string(committed) {
		t.Fatal("identifiers.gen.go is stale or hand-edited: run `make generate-telemetry`")
	}
}
