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

package generator

import "regexp"

// dataContractAssignmentRegex matches a bloblang assignment of the form
// msg.meta.data_contract = "<name>" in tag-processor defaults, conditions
// and advancedProcessing snippets.
var dataContractAssignmentRegex = regexp.MustCompile(`msg\.meta\.data_contract\s*=\s*"([^"]+)"`)

// extractDataContractsFromPipeline returns the data contract names assigned
// in any tag_processor step of the pipeline, deduped and in first-seen order.
// It returns nil when the pipeline contains no tag-processor assignment.
//
// Scanned locations per tag_processor: defaults, each condition's then clause,
// and advancedProcessing.
func extractDataContractsFromPipeline(pipeline map[string]any) []string {
	processors, ok := pipeline["processors"].([]any)
	if !ok {
		return nil
	}

	var contracts []string
	seen := make(map[string]struct{})

	appendUnique := func(name string) {
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		contracts = append(contracts, name)
	}

	for _, p := range processors {
		proc, ok := p.(map[string]any)
		if !ok {
			continue
		}

		tagProc, ok := proc["tag_processor"].(map[string]any)
		if !ok {
			continue
		}

		collectDataContracts(tagProc, appendUnique)
	}

	return contracts
}

func collectDataContracts(tagProc map[string]any, emit func(string)) {
	if defaults, ok := tagProc["defaults"].(string); ok {
		for _, match := range dataContractAssignmentRegex.FindAllStringSubmatch(defaults, -1) {
			emit(match[1])
		}
	}

	if conditions, ok := tagProc["conditions"].([]any); ok {
		for _, c := range conditions {
			cond, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if then, ok := cond["then"].(string); ok {
				for _, match := range dataContractAssignmentRegex.FindAllStringSubmatch(then, -1) {
					emit(match[1])
				}
			}
		}
	}

	if advanced, ok := tagProc["advancedProcessing"].(string); ok {
		for _, match := range dataContractAssignmentRegex.FindAllStringSubmatch(advanced, -1) {
			emit(match[1])
		}
	}
}
