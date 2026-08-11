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

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// downgradeConfigCommand is the argv[1] that selects the downgrade instead of
// starting the agent.
const downgradeConfigCommand = "downgrade-config"

// runDowngradeConfig rewrites a config into the pre-merge shape and exits.
//
// It must be run before moving an instance to a release from before the data
// contract merge. Such a release cannot decode the merged section, and rather than
// failing visibly it caches an empty config and reports nothing wrong -- so the
// instance comes up with no bridges and no error.
//
// Deliberately no reconcile, no supervisor, no signal handling: read, convert,
// write. It is run against a stopped instance, and the less it does the less there
// is to go wrong at the moment someone is already rolling back.
func runDowngradeConfig(args []string) int {
	path := config.DefaultConfigPath
	if len(args) > 0 {
		path = args[0]
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "downgrade-config: cannot read %s: %v\n", path, err)

		return 1
	}

	converted, err := config.DowngradeConfigYAML(context.Background(), data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "downgrade-config: %v\n", err)

		return 1
	}

	// Backed up first. This runs during a rollback, when the operator has enough to
	// worry about without an unrecoverable config rewrite.
	backupPath := path + ".premerge-backup"
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "downgrade-config: cannot write backup %s: %v\n", backupPath, err)

		return 1
	}

	if err := os.WriteFile(path, converted, 0o666); err != nil {
		fmt.Fprintf(os.Stderr, "downgrade-config: cannot write %s: %v\n", path, err)

		return 1
	}

	fmt.Printf("Converted %s to the pre-merge format. Original saved as %s.\n", path, backupPath)

	return 0
}
