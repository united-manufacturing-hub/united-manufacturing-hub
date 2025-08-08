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

// Package manager provides a concrete Service implementation that manages
// s6-rc services by writing service definitions, compiling them, and
// applying changes to the live system.
package manager

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// S6RCService manages services using s6-rc definitions and changeover.
type S6RCService struct {
	logger            *zap.Logger
	servicesBaseDir   string
	compiledTargetDir string
	bundleName        string
	runScriptTmpl     *template.Template
}

const (
	defaultServicesBaseDir   = "/etc/s6-overlay/s6-rc.d"
	defaultCompiledTargetDir = "/etc/s6-overlay/s6-rc/compiled"
	defaultBundleName        = "user"
)

const (
	dirPermission        = 0o750
	filePermission       = 0o600
	executablePermission = 0o755
)

// NewS6RCService constructs a new S6RCService with the provided paths.
// If any path is empty, sensible defaults are used for s6-overlay v3.
func NewS6RCService(logger *zap.Logger, servicesBaseDir, compiledTargetDir, bundleName string) *S6RCService {
	if servicesBaseDir == "" {
		servicesBaseDir = defaultServicesBaseDir
	}

	if compiledTargetDir == "" {
		compiledTargetDir = defaultCompiledTargetDir
	}

	if bundleName == "" {
		bundleName = defaultBundleName
	}

	const runTemplate = `#!/command/execlineb -P
cd /
fdmove -c 2 1
{{- if not .Args }}
{{ .Executable }}
{{- else }}
{{ .Executable }} {{ join .Args " " }}
{{- end }}`

	tmpl := template.Must(
		template.New("run").
			Funcs(template.FuncMap{"join": strings.Join}).
			Parse(runTemplate),
	)

	return &S6RCService{
		logger:            logger,
		servicesBaseDir:   servicesBaseDir,
		compiledTargetDir: compiledTargetDir,
		bundleName:        bundleName,
		runScriptTmpl:     tmpl,
	}
}

// ----- internals -----

func (s *S6RCService) writeServiceDefinition(name, executable string, argsMap map[int]string) error {
	serviceDir := filepath.Join(s.servicesBaseDir, name)
	if err := os.MkdirAll(serviceDir, dirPermission); err != nil {
		return fmt.Errorf("create service dir %s: %w", serviceDir, err)
	}

	// type
	if err := os.WriteFile(filepath.Join(serviceDir, "type"), []byte("longrun\n"), filePermission); err != nil {
		return fmt.Errorf("write type for %s: %w", name, err)
	}

	// run
	args := orderedArgs(argsMap)
	runContent, err := s.renderRunScript(executable, args)

	if err != nil {
		return fmt.Errorf("render run script for %s: %w", name, err)
	}

	runPath := filepath.Join(serviceDir, "run")

	if err := os.WriteFile(runPath, []byte(runContent), filePermission); err != nil {
		return fmt.Errorf("write run for %s: %w", name, err)
	}

	if err := os.Chmod(runPath, executablePermission); err != nil {
		return fmt.Errorf("chmod run for %s: %w", name, err)
	}

	return nil
}

func (s *S6RCService) setBundleMembership(name string, include bool) error {
	contentsDir := filepath.Join(s.servicesBaseDir, s.bundleName, "contents.d")
	if err := os.MkdirAll(contentsDir, dirPermission); err != nil {
		return fmt.Errorf("ensure bundle contents dir: %w", err)
	}

	markerPath := filepath.Join(contentsDir, name)
	if include {
		// Touch the file
		if err := os.WriteFile(markerPath, []byte("\n"), filePermission); err != nil {
			return fmt.Errorf("include %s in bundle: %w", name, err)
		}

		return nil
	}

	// Exclude
	if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("exclude %s from bundle: %w", name, err)
	}

	return nil
}

func getRandDirName() string {
	return filepath.Join(os.TempDir(), uuid.New().String())
}

func (s *S6RCService) compileAndChangeover() error {
	// Generate a unique temporary path but don't create the directory
	// s6-rc-compile wants to create the target directory itself
	tmpDir := getRandDirName()

	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := s.run("s6-rc-compile", tmpDir, s.servicesBaseDir); err != nil {
		return fmt.Errorf("s6-rc-compile failed: %w", err)
	}

	// Replace target compiled dir atomically (best-effort)
	if err := os.RemoveAll(s.compiledTargetDir); err != nil {
		return fmt.Errorf("remove compiled target: %w", err)
	}
	// Ensure target dir exists, then copy contents (not the top dir) to it.
	if err := os.MkdirAll(s.compiledTargetDir, dirPermission); err != nil {
		return fmt.Errorf("create compiled target: %w", err)
	}
	// Use cp -a to preserve modes; copy contents with trailing '/.'
	if err := s.run("cp", "-a", filepath.Clean(tmpDir)+"/.", s.compiledTargetDir); err != nil {
		return fmt.Errorf("copy compiled db: %w", err)
	}

	// Apply changeover for the bundle
	if err := s.run("s6-rc", "-u", "change", s.bundleName); err != nil {
		return fmt.Errorf("apply changeover: %w", err)
	}

	return nil
}

func (s *S6RCService) renderRunScript(executable string, args []string) (string, error) {
	// naive quoting for POC: wrap args with spaces/quotes in double quotes
	renderedArgs := make([]string, 0, len(args))

	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\n\"'") {
			escaped := strings.ReplaceAll(arg, "\"", "\\\"")
			renderedArgs = append(renderedArgs, "\""+escaped+"\"")

			continue
		}

		renderedArgs = append(renderedArgs, arg)
	}

	data := struct {
		Executable string
		Args       []string
	}{Executable: executable, Args: renderedArgs}

	var out bytes.Buffer
	if err := s.runScriptTmpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("template execute: %w", err)
	}

	return out.String(), nil
}

func orderedArgs(parameters map[int]string) []string {
	if len(parameters) == 0 {
		return nil
	}

	keys := make([]int, 0, len(parameters))

	for k := range parameters {
		keys = append(keys, k)
	}

	sort.Ints(keys)

	args := make([]string, 0, len(keys))

	for _, k := range keys {
		args = append(args, parameters[k])
	}

	return args
}

func (s *S6RCService) runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if s.logger != nil {
			s.logger.Error(
				"command failed",
				zap.String("cmd", name),
				zap.Strings("args", args),
				zap.String("stdout", strings.TrimSpace(stdout.String())),
				zap.String("stderr", strings.TrimSpace(stderr.String())),
				zap.Error(err),
			)
		}

		return stdout.String(), err
	}

	return stdout.String(), nil
}

func (s *S6RCService) run(name string, args ...string) error {
	_, err := s.runCapture(name, args...)

	return err
}

func (s *S6RCService) logWarn(msg, service string, err error) {
	if s.logger == nil {
		return
	}

	s.logger.Warn(msg, zap.String("service", service), zap.Error(err))
}
