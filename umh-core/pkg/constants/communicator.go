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

package constants

// these constants are used to make processors in communicator actions more applicable.
const (
	// nodered_js processor of benthos-umh, used for relational data.
	RelationalProcessor = "nodered_js"
	// tag_processor of benthos-umh, used for timeseries data.
	TimeseriesProcessor = "tag_processor"
)

// DetermineProcessorType returns the processor of the according category.
func DetermineProcessorType(processors any) string {
	switch p := processors.(type) {
	// case for the communicator-actions
	case map[string]struct{ Type, Data string }:
		if len(p) != 1 {
			return "custom"
		}

		for _, processor := range p {
			switch processor.Type {
			case TimeseriesProcessor:
				return TimeseriesProcessor
			case RelationalProcessor:
				return RelationalProcessor
			default:
				return "custom"
			}
		}

	// case for appending downsampler
	case []any:
		hasTagProcessor := false
		hasOtherProcessors := false

		for _, proc := range p {
			procMap, ok := proc.(map[string]any)
			if !ok {
				continue
			}

			// Check what type of processor this is
			for key := range procMap {
				if key == TimeseriesProcessor {
					hasTagProcessor = true
				} else if key != "downsampler" {
					hasOtherProcessors = true
				}

				break
			}
		}

		if hasTagProcessor {
			return TimeseriesProcessor
		}

		if len(p) == 1 && hasOtherProcessors {
			procMap, ok := p[0].(map[string]any)
			if ok {
				for key := range procMap {
					if key == RelationalProcessor {
						return RelationalProcessor
					}

					break
				}
			}
		}

		return "custom"

	default:
		return "custom"
	}

	return "custom"
}
