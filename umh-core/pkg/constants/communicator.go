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
	// RelationalProcessor is the benthos-umh processor for relational data, currently nodered_js.
	RelationalProcessor = "nodered_js"

	// TimeseriesProcessor is the benthos-umh processor for timeseries data, currently the tag_processor.
	TimeseriesProcessor = "tag_processor"

	// DownsamplerProcessor is the benthos-umh processor for the downsampler.
	DownsamplerProcessor = "downsampler"

	// CustomProcessor is a custom processing set of benthos-umh processors.
	CustomProcessor = "custom"

	// TimeseriesData indicates the pipeline processes timeseries data.
	TimeseriesData = "timeseries"

	// RelationalData indicates the pipeline processes relational data.
	RelationalData = "relational"

	// CustomData indicates the pipeline processes custom data.
	CustomData = "custom"
)

// DetermineDataType analyzes the pipeline processors to determine the core data type.
func DetermineDataType(processors []any) string {
	if len(processors) > 1 {
		return CustomData
	}

	if len(processors) == 1 {
		procMap, ok := processors[0].(map[string]any)
		if !ok {
			return CustomData
		}

		for processorType := range procMap {
			if processorType == DownsamplerProcessor {
				continue
			}

			switch processorType {
			case RelationalProcessor:
				return RelationalData
			case TimeseriesProcessor:
				return TimeseriesData
			default:
				return CustomData
			}
		}
	}

	return CustomData
}
