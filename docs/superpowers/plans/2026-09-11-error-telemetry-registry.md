# Error-telemetry registry implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give umh-core a declared set of Sentry events, so every event has one spelling, one severity and one human-readable sentence, and an unregistered event cannot be emitted.

**Architecture:** A new `umh-core/pkg/telemetry` package holds `telemetry.yaml` as the single source of truth and a generator that turns it into `identifiers.gen.go`, a nested struct tree of `Identifier` values. `deps.FSMLogger` gains one method, `Sentry`, which takes an `Identifier` instead of a message string and reads the log level from it. The existing `SentryHook` needs no change to route or group these events; it already tags `event_name` from the message. All 165 existing call sites convert to the new method, and the two old methods are deleted at the end.

**Tech Stack:** Go 1.27, `gopkg.in/yaml.v3` (already a direct dependency), Ginkgo v2 + Gomega for specs, `go:generate` plus a Makefile target for generation.

**Spec:** `docs/superpowers/specs/2026-09-10-error-telemetry-registry-design.md`

## Global Constraints

- Every new source file starts with the repo's Apache 2.0 header, copied verbatim from any existing file (13 lines, beginning `// Copyright 2025 UMH Systems GmbH`). Lefthook's `fix-license-header` job injects it otherwise, which breaks Task 5.
- Branch is `error-telemetry-registry`, cut from `staging`. Never push to `staging` directly.
- `umh-core/CHANGELOG.md` needs an entry under `## Unreleased`; CI fails a code change without one. Add it in Task 17.
- Tests are Ginkgo v2 specs in a `_test` package with a `TestXxx` bootstrap calling `RunSpecs`. Do not commit focused specs: CI runs `ginkgo -r --fail-on-focused`.
- Neither static analyser works on this machine: `golangci-lint` 2.6.2 starts but its type-checker cannot decode go1.27 export data, so it reports spurious `typecheck` errors on untouched packages too, and `nilaway` fails with `package requires newer Go version go1.27`. Do not run either, and do not read their output as a finding. Per-task verification is `go build ./...`, `go vet -tags=test ./...` and `go test -race -tags=test ./<scope>/...`. CI runs both analysers.
- Tag format, fixed: `^[a-z0-9_]+(::[a-z0-9_]+)+$`.
- `Tag` and `Severity` are stable for the life of an entry. Both feed the Sentry fingerprint, so editing either re-groups the issue and orphans its history. `Brief` is safe to edit.

---

## File Structure

| File | Responsibility |
|---|---|
| `umh-core/pkg/telemetry/identifier.go` | The `Identifier` and `Severity` types. No dependencies, so any package can import it. |
| `umh-core/pkg/telemetry/telemetry.yaml` | Single source of truth: every event, its brief, its severity. Hand-edited. |
| `umh-core/pkg/telemetry/gen.go` | Holds the `go:generate` directive and nothing else. |
| `umh-core/pkg/telemetry/generate/main.go` | The generator: YAML in, Go out. Pure transformation, sorted emission. |
| `umh-core/pkg/telemetry/identifiers.gen.go` | Generated. Nested struct tree of `Identifier` values, one exported var per domain. |
| `umh-core/pkg/telemetry/generate/generator_test.go` | Generator tests: tree shape, sorted output. Plain Go tests, in `package main` beside the generator. |
| `umh-core/pkg/telemetry/generate/generator_reject_test.go` | Generator tests: every rejection case. |
| `umh-core/pkg/telemetry/registry_test.go` | Registry specs over the generated tree: tag format, non-empty briefs, regeneration currency. |
| `umh-core/pkg/fsmv2/deps/logger.go` | `FSMLogger` gains `Sentry`; loses `SentryWarn`/`SentryError` in Task 16. |
| `umh-core/pkg/fsmv2/deps/logger_impl.go` | `zapLogger.Sentry`: level from severity, brief synthesized when `cause` is nil. |
| `umh-core/pkg/fsmv2/deps/logger_noop.go` | `nopLogger.Sentry`, empty body. |
| `umh-core/pkg/fsmv2/sentry/hook.go` | `IsInternalFrame` learns the new method name in Task 16. |
| `umh-core/Makefile` | `generate-telemetry` target. |

---

## Task 1: Identifier and Severity types

**Files:**
- Create: `umh-core/pkg/telemetry/identifier.go`
- Create: `umh-core/pkg/telemetry/telemetry_suite_test.go`
- Test: `umh-core/pkg/telemetry/identifier_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `telemetry.Identifier{Tag, Brief string; Severity Severity}`, `telemetry.Severity` with `SeverityWarning Severity = "warning"` and `SeverityError Severity = "error"`, and `func (i Identifier) IsZero() bool`.

- [ ] **Step 1: Write the failing test**

`umh-core/pkg/telemetry/telemetry_suite_test.go` (bootstrap, mirrors `pkg/cpuhealth/cpuhealth_suite_test.go`):

```go
package telemetry_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTelemetry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Telemetry Suite")
}
```

`umh-core/pkg/telemetry/identifier_test.go`:

```go
package telemetry_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/telemetry"
)

// An Identifier reaches the logger by value, and the logger cannot tell a
// generated one from a zero struct without asking. IsZero is that question.
var _ = Describe("Identifier", func() {
	It("reports the zero value as zero", func() {
		Expect(telemetry.Identifier{}.IsZero()).To(BeTrue())
	})

	It("reports a populated value as not zero", func() {
		id := telemetry.Identifier{
			Tag:      "cpu::read_failed",
			Brief:    "A cgroup CPU file could not be read.",
			Severity: telemetry.SeverityWarning,
		}
		Expect(id.IsZero()).To(BeFalse())
	})

	It("treats an empty tag as zero even when the brief is set", func() {
		// The tag is what reaches Sentry. A brief without one cannot be reported
		// under any name, so the logger must take the unregistered path.
		Expect(telemetry.Identifier{Brief: "something"}.IsZero()).To(BeTrue())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd umh-core && go test ./pkg/telemetry/...`
Expected: FAIL, `undefined: telemetry.Identifier`.

- [ ] **Step 3: Write minimal implementation**

`umh-core/pkg/telemetry/identifier.go` (after the 13-line licence header):

```go
// Package telemetry declares every Sentry event umh-core can report. The set
// lives in telemetry.yaml and reaches Go through identifiers.gen.go; nothing
// here is hand-written per event.
package telemetry

// Severity is an event's log level, declared once per event in telemetry.yaml.
type Severity string

const (
	// SeverityWarning reports at WARN level.
	SeverityWarning Severity = "warning"
	// SeverityError reports at ERROR level.
	SeverityError Severity = "error"
)

// Identifier is one declared event.
type Identifier struct {
	// Tag is the hierarchical event name, e.g. "cpu::read_failed::cpu_stat::missing".
	Tag string
	// Brief is one sentence on what the event means, shown under the Sentry title.
	Brief string
	// Severity is the level the event reports at.
	Severity Severity
}

// IsZero reports whether the Identifier carries no tag, which means it did not
// come from the generated tree.
func (i Identifier) IsZero() bool {
	return i.Tag == ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd umh-core && go test ./pkg/telemetry/...`
Expected: PASS, 3 specs.

- [ ] **Step 5: Commit**

```bash
git add umh-core/pkg/telemetry/identifier.go umh-core/pkg/telemetry/identifier_test.go umh-core/pkg/telemetry/telemetry_suite_test.go
git commit -m "feat(telemetry): Identifier and Severity types"
```

---

## Task 2: Generator, happy path

**Files:**
- Create: `umh-core/pkg/telemetry/generate/main.go`
- Create: `umh-core/pkg/telemetry/gen.go`
- Create: `umh-core/pkg/telemetry/telemetry.yaml` (two entries only; Task 4 fills it)
- Create: `umh-core/pkg/telemetry/identifiers.gen.go` (generated)
- Modify: `umh-core/Makefile`
- Test: `umh-core/pkg/telemetry/generate/generator_test.go`

**Interfaces:**
- Consumes: `telemetry.Identifier`, `telemetry.Severity` from Task 1.
- Produces: `func Generate(yamlBytes []byte, pkgName string) ([]byte, error)` in package `main` of `generate/`, exported for its test; the CLI wrapper reads `-in` and writes `-out`. Also produces the emitted shape later tasks call: one exported var per domain, e.g. `telemetry.Cpu.ReadFailed`.

**Note on naming:** the spec's rule is title-case per underscore-separated part, so domain `cpu` emits `Cpu`, not `CPU`. `golangci-lint` may object under `stylecheck`/`revive` initialism rules. Run lint in Step 4 before committing. If it objects, stop and report rather than inventing an initialism map: that changes every call site's spelling and is the human's call.

- [ ] **Step 1: Write the failing test**

`umh-core/pkg/telemetry/generate/generator_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

const twoEntryYAML = `
cpu:
  read_failed:
    brief: A cgroup CPU file could not be read; the measurement continues.
    severity: warning
transport:
  push::persistent_failure:
    brief: Outbound pushes keep failing; the instance may look offline.
    severity: error
`

func TestGenerateEmitsOneVarPerDomain(t *testing.T) {
	out, err := Generate([]byte(twoEntryYAML), "telemetry")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"// Code generated from telemetry.yaml. DO NOT EDIT.",
		"Copyright 2025 UMH Systems GmbH",
		"package telemetry",
		`var Cpu = cpuNode{`,
		`var Transport = transportNode{`,
		`Tag: "cpu::read_failed"`,
		`Tag: "transport::push::persistent_failure"`,
		`Severity: SeverityWarning`,
		`Severity: SeverityError`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestGenerateNestsOnSeparator(t *testing.T) {
	out, _ := Generate([]byte(twoEntryYAML), "telemetry")
	got := string(out)
	// push::persistent_failure is two segments, so Push is a struct holding
	// PersistentFailure, not a field named PushPersistentFailure.
	if !strings.Contains(got, "Push transportPushNode") {
		t.Error("expected a nested node type for the push segment")
	}
	if strings.Contains(got, "PushPersistentFailure") {
		t.Error("segments were flattened instead of nested")
	}
}

func TestGenerateIsSortedAndDeterministic(t *testing.T) {
	first, _ := Generate([]byte(twoEntryYAML), "telemetry")
	second, _ := Generate([]byte(twoEntryYAML), "telemetry")
	if string(first) != string(second) {
		t.Fatal("two runs over identical input differ")
	}
	got := string(first)
	if strings.Index(got, "var Cpu") > strings.Index(got, "var Transport") {
		t.Error("domains are not emitted in sorted order")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd umh-core && go test ./pkg/telemetry/generate/...`
Expected: FAIL, `undefined: Generate`.

- [ ] **Step 3: Write minimal implementation**

`umh-core/pkg/telemetry/generate/main.go`. Structure it as: parse YAML into `map[string]map[string]entry`, build a tree keyed by segment, then walk the tree in sorted key order emitting node types and vars. Required behaviour:

```go
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const licenceHeader = `// Copyright 2025 UMH Systems GmbH
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
`

type entry struct {
	Brief    string `yaml:"brief"`
	Severity string `yaml:"severity"`
}

// node is one level of the emitted tree: either a leaf (an entry) or a branch.
type node struct {
	children map[string]*node
	leaf     *entry
	tag      string
}

// Generate turns telemetry.yaml bytes into identifiers.gen.go bytes. Emission is
// sorted by key at every level, because a YAML mapping decodes into an unordered
// Go map and the regeneration test compares bytes.
func Generate(yamlBytes []byte, pkgName string) ([]byte, error) {
	var doc map[string]map[string]entry
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return nil, fmt.Errorf("parse telemetry.yaml: %w", err)
	}

	roots, err := buildTree(doc)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString(licenceHeader)
	out.WriteString("\n// Code generated from telemetry.yaml. DO NOT EDIT.\n\n")
	fmt.Fprintf(&out, "package %s\n", pkgName)

	for _, domain := range sortedKeys(roots) {
		emitNodeTypes(&out, goName(domain), roots[domain])
		emitVar(&out, domain, roots[domain])
	}

	return out.Bytes(), nil
}

func main() {
	in := flag.String("in", "telemetry.yaml", "path to the source YAML")
	out := flag.String("out", "identifiers.gen.go", "path to the generated Go file")
	pkg := flag.String("package", "telemetry", "package name to emit")
	flag.Parse()

	src, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	generated, err := Generate(src, *pkg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Written only after a successful generate, so a malformed entry leaves the
	// previous file intact rather than truncating it.
	if err := os.WriteFile(*out, generated, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

Write the helpers out rather than sketching them. `buildTree` splits each key on `::` and records the full tag on the leaf. `goName` renders one segment as a Go identifier with no initialism special-casing, so `cpu` becomes `Cpu`. `emitNodeTypes` declares a struct type per branch depth-first, so a type exists before the type embedding it. `emitVar` writes the exported per-domain var. `sortedKeys` is what imposes order over YAML's unordered map, and every recursion uses it. Pass the assembled bytes through `format.Source` from `go/format` before returning, so emitted indentation need not be exact:

```go
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("generated source does not parse: %w", err)
	}

	return formatted, nil
```

Leaf emission is the one line that has to be exact, because Task 5 compares bytes:

```go
	fmt.Fprintf(out, "%s%s: Identifier{Tag: %q, Brief: %q, Severity: Severity%s},\n",
		indent, goName(key), child.tag, child.leaf.Brief, goName(child.leaf.Severity))
```

`buildTree` returns errors only for the rejection cases Task 3 adds; here it needs the happy path only.

`umh-core/pkg/telemetry/gen.go`:

```go
package telemetry

//go:generate go run ./generate -in telemetry.yaml -out identifiers.gen.go -package telemetry
```

`umh-core/pkg/telemetry/telemetry.yaml` — the two entries from the test, with a header comment stating the stability rule:

```yaml
# Every Sentry event umh-core can report. Editing this file is how an event comes
# to exist; identifiers.gen.go is generated from it by `make generate-telemetry`.
#
# Tag and severity are stable for the life of an entry: both feed the Sentry
# fingerprint, so changing either starts a new issue and orphans the old one's
# history. Brief is safe to edit.
cpu:
  read_failed:
    brief: A cgroup CPU file could not be read; the measurement continues.
    severity: warning
transport:
  push::persistent_failure:
    brief: Outbound pushes keep failing; the instance may look offline.
    severity: error
```

`umh-core/Makefile`, next to the other tooling targets:

```make
generate-telemetry:
	cd pkg/telemetry && go generate .
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd umh-core
go test ./pkg/telemetry/generate/...          # 3 tests pass
make generate-telemetry                        # writes identifiers.gen.go
go build ./pkg/telemetry/...                   # generated file compiles
```

- [ ] **Step 5: Commit**

```bash
git add umh-core/pkg/telemetry umh-core/Makefile
git commit -m "feat(telemetry): generate identifiers from telemetry.yaml"
```

---

## Task 3: Generator rejections

**Files:**
- Modify: `umh-core/pkg/telemetry/generate/main.go`
- Test: `umh-core/pkg/telemetry/generate/generator_reject_test.go`

**Interfaces:**
- Consumes: `Generate` from Task 2.
- Produces: no new signature. `Generate` returns a non-nil error naming the offending key for every case below, and `main` exits non-zero without writing.

Covers spec edge cases 4, 5, 6, 6b, 7, 8, 12c.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"strings"
	"testing"
)

func TestGenerateRejects(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantKey string
	}{
		{"missing brief", "cpu:\n  read_failed:\n    severity: warning\n", "read_failed"},
		{"missing severity", "cpu:\n  read_failed:\n    brief: x\n", "read_failed"},
		{"unknown severity", "cpu:\n  a:\n    brief: x\n    severity: fatal\n", "fatal"},
		{"duplicate tag", "cpu:\n  a::b:\n    brief: x\n    severity: warning\n  a::b:\n    brief: y\n    severity: error\n", "a::b"},
		{"go path collision, leaf vs branch", "cpu:\n  push:\n    brief: x\n    severity: warning\n  push::failed:\n    brief: y\n    severity: warning\n", "push"},
		{"go path collision, underscores", "cpu:\n  a_b:\n    brief: x\n    severity: warning\n  a__b:\n    brief: y\n    severity: warning\n", "a__b"},
		{"leading separator", "cpu:\n  ::a:\n    brief: x\n    severity: warning\n", "::a"},
		{"trailing separator", "cpu:\n  a:::\n    brief: x\n    severity: warning\n", "a:::"},
		{"illegal character", "cpu:\n  a-b:\n    brief: x\n    severity: warning\n", "a-b"},
		{"go keyword segment", "cpu:\n  range:\n    brief: x\n    severity: warning\n", "range"},
		{"leading digit", "cpu:\n  1st_try:\n    brief: x\n    severity: warning\n", "1st_try"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Generate([]byte(tc.yaml), "telemetry")
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantKey) {
				t.Errorf("error %q does not name %q", err, tc.wantKey)
			}
		})
	}
}

func TestGenerateAcceptsEmptyDomain(t *testing.T) {
	// Edge case 9: an empty domain is a work-in-progress edit, not an error.
	out, err := Generate([]byte("cpu:\n"), "telemetry")
	if err != nil {
		t.Fatalf("empty domain rejected: %v", err)
	}
	if strings.Contains(string(out), "var Cpu") {
		t.Error("emitted a var for a domain with no entries")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd umh-core && go test ./pkg/telemetry/generate/ -run TestGenerateRejects -v`
Expected: FAIL on most subtests with "expected an error, got nil".

- [ ] **Step 3: Write minimal implementation**

In `buildTree` and `goName`, add, in this order, each returning an error naming the key:

1. `brief` empty, or `severity` empty.
2. `severity` not `warning` and not `error`; the message names the bad value.
3. The composed tag does not match `^[a-z0-9_]+(::[a-z0-9_]+)+$`. This one catches leading, trailing and doubled separators plus illegal characters.
4. The composed tag already exists; the message names the tag.
5. A segment whose `goName` is a Go keyword (`token.IsKeyword`), or starts with a digit.
6. Two entries whose full Go path collides. Track `map[string]string` from Go path to the YAML key that claimed it; on a second claim, return an error naming both keys. A leaf claiming a path a branch already holds is the same check.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd umh-core
go test ./pkg/telemetry/generate/...        # 11 subtests + 4 tests pass
```

- [ ] **Step 5: Commit**

```bash
git add umh-core/pkg/telemetry/generate
git commit -m "feat(telemetry): reject entries the generator cannot compile"
```

---

## Task 4: telemetry.yaml, the ~149 real entries

**Files:**
- Modify: `umh-core/pkg/telemetry/telemetry.yaml`
- Modify: `umh-core/pkg/telemetry/identifiers.gen.go` (regenerated)

**Interfaces:**
- Consumes: the generator from Tasks 2 and 3.
- Produces: the vars every conversion task calls, e.g. `telemetry.Supervisor.Worker.AddRejected`.

**This task needs a human and splits by subsystem.** Authoring ~149 briefs and choosing ~149 severities requires knowing what each event means. Do 4a through 4f as separate commits, each one subsystem, and stop for the human on anything unclear rather than inventing a brief. The brief is the Sentry subtitle, so a wrong one is visible in every alert.

List the current names and their levels first:

```bash
cd umh-core
grep -rn "SentryWarn(\|SentryError(" --include="*.go" pkg/ | grep -v _test
```

| Sub-task | Subsystem | Sites |
|---|---|---|
| 4a | `pkg/fsmv2/supervisor` top level | 67 |
| 4b | `pkg/fsmv2/supervisor/internal/*` | 27 |
| 4c | `pkg/fsmv2/workers/*` | 28 |
| 4d | `pkg/communicator/*` | 22 |
| 4e | `pkg/config` | 12 |
| 4f | `pkg/fsmv2/cpu`, `pkg/cse`, remaining `pkg/fsmv2` | 9 |

Three decisions belong to the human, not to whoever runs the task:

1. **Eight prose names** need reading to learn what event each is. They are `build spec failed, will retry next tick`, `config watch: failed to read config`, `cpu watch: upsert failed`, `cpu: startup cgroup snapshot failed; quota signals omitted`, `failed to load persistence observed state`, `historian watch: upsert failed`, `unknown action type`, `upsert failed, will retry next tick`. The `:` and `;` in them are illegal in a tag.
2. **Three dual-severity names** are reported today at both warn and error: `collector_restart_failed`, `desired_state_load_failed`, `shutdown_request_failed`. One declared severity changes the level at some call sites. Record which level wins and why.
3. **Four computed messages** are not string literals at all. Find them with the scan above and decide whether each becomes one entry or several.

- [ ] **Step 1 (per sub-task): add that subsystem's entries to telemetry.yaml**
- [ ] **Step 2: regenerate and build**

```bash
cd umh-core && make generate-telemetry && go build ./pkg/telemetry/...
```

- [ ] **Step 3: run the registry specs from Task 5 (they must stay green as entries land)**

```bash
cd umh-core && go test ./pkg/telemetry/...
```

- [ ] **Step 4: commit per sub-task**

```bash
git add umh-core/pkg/telemetry/telemetry.yaml umh-core/pkg/telemetry/identifiers.gen.go
git commit -m "feat(telemetry): register the supervisor events"   # adjust per sub-task
```

---

## Task 5: Registry and regeneration specs

**Files:**
- Test: `umh-core/pkg/telemetry/registry_test.go`

**Interfaces:**
- Consumes: the generated tree, the generator binary.
- Produces: nothing; this task is only tests.

Covers spec properties 1, 2, 3 and edge cases 10, 11, 11b.

- [ ] **Step 1: Write the failing test**

```go
package telemetry_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/telemetry"
)

var tagFormat = regexp.MustCompile(`^[a-z0-9_]+(::[a-z0-9_]+)+$`)

// walk collects every Identifier in the generated tree by reflection, so the
// specs cover entries added later without being edited.
func walk(v reflect.Value, out *[]telemetry.Identifier) {
	idType := reflect.TypeOf(telemetry.Identifier{})
	if v.Type() == idType {
		*out = append(*out, v.Interface().(telemetry.Identifier))

		return
	}

	if v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField(); i++ {
			walk(v.Field(i), out)
		}
	}
}

var _ = Describe("the generated registry", func() {
	var all []telemetry.Identifier

	BeforeEach(func() {
		all = nil
		// Add each domain var here as Task 4 introduces it.
		walk(reflect.ValueOf(telemetry.Cpu), &all)
		walk(reflect.ValueOf(telemetry.Transport), &all)
	})

	It("holds at least one entry", func() {
		// Without this the three specs below pass over an empty slice.
		Expect(all).NotTo(BeEmpty())
	})

	It("gives every entry a tag in the declared format", func() {
		for _, id := range all {
			Expect(tagFormat.MatchString(id.Tag)).To(BeTrue(), "tag %q", id.Tag)
		}
	})

	It("gives every entry a brief and a known severity", func() {
		for _, id := range all {
			Expect(id.Brief).NotTo(BeEmpty(), "tag %q has no brief", id.Tag)
			Expect(id.Severity).To(BeElementOf(telemetry.SeverityWarning, telemetry.SeverityError), "tag %q", id.Tag)
		}
	})

	It("gives every entry a unique tag", func() {
		seen := map[string]bool{}
		for _, id := range all {
			Expect(seen[id.Tag]).To(BeFalse(), "duplicate tag %q", id.Tag)
			seen[id.Tag] = true
		}
	})
})

// TestGeneratedFileIsCurrent is a plain Go test rather than a spec because it
// shells out: it regenerates into a temp file and compares bytes, so an edited
// YAML with a forgotten `make generate-telemetry` fails here rather than shipping.
func TestGeneratedFileIsCurrent(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "identifiers.gen.go")

	cmd := exec.Command("go", "run", "./generate", "-in", "telemetry.yaml", "-out", out, "-package", "telemetry")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("regeneration failed: %v\n%s", err, combined)
	}

	fresh, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	committed, err := os.ReadFile("identifiers.gen.go")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.EqualFold(string(fresh), string(committed)) {
		t.Fatal("identifiers.gen.go is stale or hand-edited: run `make generate-telemetry`")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Temporarily add an entry to `telemetry.yaml` without regenerating, then:

Run: `cd umh-core && go test ./pkg/telemetry/ -run TestGeneratedFileIsCurrent -v`
Expected: FAIL with "identifiers.gen.go is stale". Revert the YAML edit afterwards.

- [ ] **Step 3: No implementation needed** — Tasks 2 and 3 already satisfy these specs. If any fail, the defect is in the generator, not here.

- [ ] **Step 4: Run the whole package**

```bash
cd umh-core && go test ./pkg/telemetry/...
```

- [ ] **Step 5: Commit**

```bash
git add umh-core/pkg/telemetry/registry_test.go
git commit -m "test(telemetry): pin the tag format and the generated file's currency"
```

---

## Task 6: Sentry on FSMLogger and every implementor

**Files:**
- Modify: `umh-core/pkg/fsmv2/deps/logger.go`
- Modify: `umh-core/pkg/fsmv2/deps/logger_impl.go`
- Modify: `umh-core/pkg/fsmv2/deps/logger_noop.go`
- Modify: `umh-core/pkg/fsmv2/integration/test_logger.go`
- Modify: `umh-core/pkg/communicator/actions/actions_recovery_test.go` (3 fake types)
- Modify: `umh-core/pkg/communicator/actions/edit-protocolconverter_test.go`
- Modify: `umh-core/pkg/fsmv2/workers/transport/action/authenticate_test.go`
- Modify: `umh-core/pkg/fsmv2/supervisor/panic_recovery_test.go`
- Modify: `umh-core/pkg/fsmv2/supervisor/internal/collection/collector_panic_test.go`
- Modify: `umh-core/pkg/fsmv2/supervisor/internal/collection/collector_logseverity_test.go`
- Test: `umh-core/pkg/fsmv2/deps/logger_sentry_test.go`

**Interfaces:**
- Consumes: `telemetry.Identifier`, `telemetry.Severity`.
- Produces: `Sentry(id telemetry.Identifier, feature Feature, hierarchyPath string, cause error, fields ...Field)` on `FSMLogger`.

**Everything in one commit.** Adding a method to the interface breaks every implementor the moment it lands, so the fakes cannot wait for Task 16.

- [ ] **Step 1: Write the failing test**

```go
package deps_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/telemetry"
)

// Level and message are what the Sentry hook reads: it forwards warn and above,
// takes the message as event_name, and renders the error as the subtitle.
var _ = Describe("FSMLogger.Sentry", func() {
	warning := telemetry.Identifier{Tag: "cpu::read_failed", Brief: "A CPU file could not be read.", Severity: telemetry.SeverityWarning}
	failure := telemetry.Identifier{Tag: "cpu::sample_failed", Brief: "No CPU sample could be taken.", Severity: telemetry.SeverityError}

	It("logs a warning-severity identifier at warn, with the tag as the message", func() {
		entries, logger := observedLogger()   // existing helper in this suite
		logger.Sentry(warning, deps.FeatureSupportCPU, "root/cpu-1(cpu)", nil)

		Expect(entries.All()).To(HaveLen(1))
		Expect(entries.All()[0].Level.String()).To(Equal("warn"))
		Expect(entries.All()[0].Message).To(Equal("cpu::read_failed"))
	})

	It("logs an error-severity identifier at error", func() {
		entries, logger := observedLogger()
		logger.Sentry(failure, deps.FeatureSupportCPU, "", errors.New("boom"))

		Expect(entries.All()[0].Level.String()).To(Equal("error"))
	})

	It("synthesizes the brief when the caller has no error", func() {
		// Without this the event reaches Sentry with no exception and no readable
		// sentence, which is the hole the registry exists to close.
		entries, logger := observedLogger()
		logger.Sentry(warning, deps.FeatureSupportCPU, "", nil)

		fields := fieldMap(entries.All()[0])   // existing helper in this suite
		Expect(fields).To(HaveKeyWithValue("error", "A CPU file could not be read."))
	})

	It("keeps the caller's error when there is one", func() {
		entries, logger := observedLogger()
		logger.Sentry(failure, deps.FeatureSupportCPU, "", errors.New("boom"))

		Expect(fieldMap(entries.All()[0])).To(HaveKeyWithValue("error", "boom"))
	})

	It("carries feature, hierarchy_path and the caller's fields", func() {
		entries, logger := observedLogger()
		logger.Sentry(warning, deps.FeatureSupportCPU, "root/cpu-1(cpu)", nil, deps.String("path", "/sys/fs/cgroup/cpu.stat"))

		fields := fieldMap(entries.All()[0])
		Expect(fields).To(HaveKeyWithValue("feature", "support_cpu"))
		Expect(fields).To(HaveKeyWithValue("hierarchy_path", "root/cpu-1(cpu)"))
		Expect(fields).To(HaveKeyWithValue("path", "/sys/fs/cgroup/cpu.stat"))
	})

	It("omits hierarchy_path when empty", func() {
		entries, logger := observedLogger()
		logger.Sentry(warning, deps.FeatureSupportCPU, "", nil)

		Expect(fieldMap(entries.All()[0])).NotTo(HaveKey("hierarchy_path"))
	})
})
```

Read `pkg/fsmv2/deps/logger_test.go` first and reuse its existing observed-logger helpers; if they are named differently, use the existing names rather than adding new ones.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd umh-core && go test ./pkg/fsmv2/deps/...`
Expected: FAIL, `logger.Sentry undefined`.

- [ ] **Step 3: Write minimal implementation**

`logger.go`, in the interface:

```go
	// Sentry logs one declared event. The level comes from id.Severity, so an
	// event's severity is declared once, in telemetry.yaml. Pass nil for cause
	// when the event carries no error: the brief is used instead, so the Sentry
	// event always has a readable sentence.
	Sentry(id telemetry.Identifier, feature Feature, hierarchyPath string, cause error, fields ...Field)
```

`logger_impl.go`:

```go
func (l *zapLogger) Sentry(id telemetry.Identifier, feature Feature, hierarchyPath string, cause error, fields ...Field) {
	if cause == nil {
		cause = errors.New(id.Brief)
	}

	allFields := make([]Field, 0, 3+len(fields))
	allFields = append(allFields, Field{Key: "feature", Value: string(feature)})

	if hierarchyPath != "" {
		allFields = append(allFields, Field{Key: "hierarchy_path", Value: hierarchyPath})
	}

	// Bare value, not wrapped: SugaredLogger.sweetenFields detects the error
	// interface and produces the zapcore.ErrorType that the Sentry hook's
	// ExtractErrorFromFields looks for.
	allFields = append(allFields, Field{Key: "error", Value: cause})
	allFields = append(allFields, fields...)

	// Anything other than SeverityWarning reports at error. The hook forwards
	// warn and above, so a level below warn would silence the event and the
	// unrecognised severity with it.
	if id.Severity == telemetry.SeverityWarning {
		l.sugar.Warnw(id.Tag, fieldsToArgs(l.baseFields, allFields)...)

		return
	}

	l.sugar.Errorw(id.Tag, fieldsToArgs(l.baseFields, allFields)...)
}
```

`logger_noop.go`:

```go
func (l *nopLogger) Sentry(_ telemetry.Identifier, _ Feature, _ string, _ error, _ ...Field) {}
```

Every fake gets the same empty or recording body, matching whatever that fake does for `SentryWarn` today.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd umh-core
go test ./pkg/fsmv2/deps/...
go build ./... && go test ./pkg/fsmv2/... ./pkg/communicator/...
```

- [ ] **Step 5: Commit**

```bash
git add umh-core/pkg/fsmv2/deps umh-core/pkg/fsmv2/integration umh-core/pkg/communicator/actions umh-core/pkg/fsmv2/workers/transport/action umh-core/pkg/fsmv2/supervisor
git commit -m "feat(deps): Sentry method taking a declared identifier"
```

---

## Task 7: Unregistered identifier guard

**Files:**
- Modify: `umh-core/pkg/fsmv2/deps/logger_impl.go`
- Modify: `umh-core/pkg/telemetry/telemetry.yaml` (one entry)
- Test: `umh-core/pkg/fsmv2/deps/logger_sentry_test.go`

**Interfaces:**
- Consumes: `Identifier.IsZero` from Task 1.
- Produces: `telemetry.Telemetry.UnregisteredIdentifier`, the fixed entry the guard reports under.

Covers spec edge case 1 and property 4.

- [ ] **Step 1: Write the failing test**

```go
	It("reports a zero identifier as a defect rather than an empty event", func() {
		// A zero Identifier is constructible even though the generated tree never
		// yields one. Emitting it would put a blank event_name into Sentry, which
		// groups every such bug into one unreadable issue.
		entries, logger := observedLogger()
		logger.Sentry(telemetry.Identifier{}, deps.FeatureFSMv2, "root", nil, deps.String("caller", "x"))

		Expect(entries.All()).To(HaveLen(1))
		Expect(entries.All()[0].Message).To(Equal("telemetry::unregistered_identifier"))
		Expect(entries.All()[0].Level.String()).To(Equal("error"))
		Expect(fieldMap(entries.All()[0])).To(HaveKeyWithValue("caller", "x"))
	})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd umh-core && go test ./pkg/fsmv2/deps/ -v`
Expected: FAIL, message is `""` rather than the fixed tag.

- [ ] **Step 3: Write minimal implementation**

Add the YAML entry, regenerate, then guard at the top of `zapLogger.Sentry`:

```yaml
telemetry:
  unregistered_identifier:
    brief: A zero-value telemetry.Identifier reached the logger; the call site is a bug.
    severity: error
```

```go
	if id.IsZero() {
		id = telemetry.Telemetry.UnregisteredIdentifier
	}
```

`telemetry.Telemetry` reads badly, and it is exactly what the spec's fixed tag `telemetry::unregistered_identifier` produces under the naming rule. Leave it: the tag is a Sentry grouping key the spec pinned, so renaming the domain to prettify the Go path changes the tag. Raise it with the human if it grates.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd umh-core && make generate-telemetry && go test ./pkg/fsmv2/deps/...
```

- [ ] **Step 5: Commit**

```bash
git add umh-core/pkg/telemetry umh-core/pkg/fsmv2/deps
git commit -m "feat(deps): report an unregistered identifier instead of a blank event"
```

---

## Tasks 8-15: convert the call sites

Same shape for every task; only the scope changes. Each is a `refactor` commit with no new test, **except** where the scope contains one of the three dual-severity names, which is a behaviour change and takes a test plus a `feat` commit.

| Task | Scope | Sites | Notes |
|---|---|---|---|
| 8 | `pkg/fsmv2/cpu` | 1 | Smallest, do it first to settle the pattern. |
| 9 | `pkg/config` | 12 | |
| 10 | `pkg/communicator` | 22 | |
| 11 | `pkg/fsmv2/workers` | 28 | |
| 12 | `supervisor/reconciliation.go` | 41 | Largest single file. |
| 13 | `supervisor/api.go`, `supervisor/lifecycle.go` | 26 | |
| 14 | `supervisor/internal/{collection,execution,health}` | 27 | Holds `collector_restart_failed`: behaviour change, needs a test. |
| 15 | remaining `pkg/fsmv2`, `pkg/cse` | 8 | |

**The transformation**, in both directions:

```go
// before, warn
d.GetLogger().SentryWarn(deps.FeatureSupportCPU, path, "cpu_stat_read_failed", deps.String("path", p))
// after
d.GetLogger().Sentry(telemetry.Cpu.StatReadFailed, deps.FeatureSupportCPU, path, nil, deps.String("path", p))

// before, error
d.GetLogger().SentryError(deps.FeatureFSMv2, path, err, "snapshot_load_failed", deps.String("worker", w))
// after
d.GetLogger().Sentry(telemetry.Supervisor.SnapshotLoadFailed, deps.FeatureFSMv2, path, err, deps.String("worker", w))
```

The message argument becomes the identifier and moves to the front; the error argument moves from third to fourth; `nil` fills it where `SentryWarn` had none. `feature`, `hierarchyPath` and the trailing fields are untouched.

- [ ] **Step 1: list the sites in scope**

```bash
cd umh-core && grep -rn "SentryWarn(\|SentryError(" --include="*.go" <scope> | grep -v _test
```

- [ ] **Step 2: convert each one**, using the identifier registered for that name in Task 4. If a name has no entry, stop: Task 4 is incomplete for this subsystem.

- [ ] **Step 3: verify nothing else changed**

```bash
cd umh-core
go build ./... && go test ./<scope>/...
git diff --stat                     # only the expected files
grep -rn "SentryWarn(\|SentryError(" --include="*.go" <scope> | grep -v _test | wc -l   # expect 0
```

- [ ] **Step 4: for Task 14 only, add the level test before converting**

`collector_restart_failed` is logged at both warn and error today. Write a spec asserting the level the human chose in Task 4, watch it fail against the current code, then convert.

- [ ] **Step 5: Commit**

```bash
git add <scope>
git commit -m "refactor(<scope>): report through the telemetry registry"
```

---

## Task 16: Delete the old methods and fix the frame filter

**Files:**
- Modify: `umh-core/pkg/fsmv2/deps/logger.go`
- Modify: `umh-core/pkg/fsmv2/deps/logger_impl.go`
- Modify: `umh-core/pkg/fsmv2/deps/logger_noop.go`
- Modify: `umh-core/pkg/fsmv2/sentry/hook.go:496`
- Modify: `umh-core/pkg/fsmv2/deps/logger_test.go` and every fake from Task 6
- Test: `umh-core/pkg/fsmv2/sentry/hook_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: an `FSMLogger` with one Sentry method.

- [ ] **Step 1: Write the failing test**

```go
	It("filters the new Sentry method out of the stack frames", func() {
		// IsInternalFrame names SentryWarn and SentryError today. Rename the
		// method and the logger's own frame becomes the innermost app frame, so
		// Sentry reports the logger as the culprit for every event instead of
		// the code that reported it.
		frame := sentry.Frame{Function: "deps.(*zapLogger).Sentry", AbsPath: "/src/umh-core/pkg/fsmv2/deps/logger_impl.go"}
		Expect(fsmv2sentry.IsInternalFrame(frame)).To(BeTrue())
	})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd umh-core && go test ./pkg/fsmv2/sentry/ -v`
Expected: FAIL, `IsInternalFrame` returns false.

- [ ] **Step 3: Write minimal implementation**

In `IsInternalFrame`, replace the two old names with one check that matches the new method, keeping the rest of the list intact:

```go
	if strings.Contains(frame.Function, "captureToSentry") ||
		strings.Contains(frame.Function, "SentryHook.Write") ||
		strings.Contains(frame.Function, "zapLogger).Sentry") ||
		strings.Contains(frame.Function, "zapcore.") {
		return true
	}
```

Then delete `SentryWarn` and `SentryError` from the interface and all implementors, and update the tests that assert their message strings — `pkg/fsmv2/deps/logger_test.go` asserts `reconciliation slow` and a bare `warn`, both of which disappear with the methods.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd umh-core
go build ./... && go test ./... 2>&1 | tail -20
grep -rn "SentryWarn(\|SentryError(" --include="*.go" pkg/ cmd/ | wc -l   # expect 0
```

- [ ] **Step 5: Commit**

```bash
git add umh-core
git commit -m "refactor(deps): delete SentryWarn and SentryError"
```

---

## Task 17: Capstone and changelog

**Files:**
- Test: `umh-core/pkg/fsmv2/deps/logger_capstone_test.go`
- Modify: `umh-core/CHANGELOG.md`

**Interfaces:**
- Consumes: everything.
- Produces: nothing.

Covers spec property 7: the whole path, from a generated identifier to an intercepted Sentry event.

- [ ] **Step 1: Write the failing test**

Wire a real `fsmv2sentry.NewSentryHook` around a zap core, as `pkg/fsmv2/cpu/cpu_read_report_test.go` does, call `Sentry` with a generated identifier, and assert on the intercepted event:

```go
	It("delivers a generated identifier to the hook as tag, level and brief", func() {
		// The unit specs prove each piece. This one proves the pieces are wired:
		// an assertion on the hook is the only one that fails if the hook stops
		// seeing these events at all.
		events, logger := hookedLogger()
		logger.Sentry(telemetry.Cpu.ReadFailed, deps.FeatureSupportCPU, "root/cpu-1(cpu)", nil)

		Expect(*events).To(HaveLen(1))
		Expect((*events)[0].Tags).To(HaveKeyWithValue("event_name", "cpu::read_failed"))
		Expect((*events)[0].Tags).To(HaveKeyWithValue("feature", "support_cpu"))
		Expect((*events)[0].Level).To(Equal(sentry.LevelWarning))
		Expect((*events)[0].Exception[0].Type).To(Equal("cpu::read_failed"))
		Expect((*events)[0].Exception[0].Value).To(Equal(telemetry.Cpu.ReadFailed.Brief))
	})
```

- [ ] **Step 2: Run test to verify it fails**

Comment out the `errors.New(id.Brief)` line in `logger_impl.go`, run, and confirm the `Exception[0].Value` assertion fails. Restore it.

Run: `cd umh-core && go test ./pkg/fsmv2/deps/ -v`
Expected: PASS once restored.

- [ ] **Step 3: Add the changelog entry**

Under `## Unreleased` in `umh-core/CHANGELOG.md`, following the `changelog-writing` skill.

- [ ] **Step 4: Full verification**

```bash
cd umh-core
go vet -tags=test ./...
go test -race -tags=test ./... 2>&1 | tail -20
ginkgo -r --fail-on-focused --dry-run 2>&1 | tail -3
```

- [ ] **Step 5: Commit**

```bash
git add umh-core
git commit -m "test(telemetry): capstone from identifier to Sentry event"
```
